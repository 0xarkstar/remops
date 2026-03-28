package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/transport"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type checkStatus string

const (
	statusPass checkStatus = "PASS"
	statusWarn checkStatus = "WARN"
	statusFail checkStatus = "FAIL"
)

type checkResult struct {
	Name   string
	Status checkStatus
	Detail string
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run health checks on remops configuration and connectivity",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	var results []checkResult

	// a. Config file found and valid
	results = append(results, checkConfig())

	if cfg != nil {
		tr := transport.NewSSHTransport(cfg)
		defer tr.Close()

		// b. Each host reachable (SSH ping)
		for name := range cfg.Hosts {
			results = append(results, checkHostPing(cmd, tr, name))
		}

		// c. Docker installed on each host
		for name := range cfg.Hosts {
			results = append(results, checkDockerInstalled(cmd, tr, name))
		}

		// d. Docker Compose installed on each host
		for name := range cfg.Hosts {
			results = append(results, checkDockerComposeInstalled(cmd, tr, name))
		}
	}

	// d. SSH key permissions
	results = append(results, checkSSHKeyPerms())

	// e. Telegram bot reachable (if approval configured)
	if cfg != nil && cfg.Approval != nil && cfg.Approval.Method == "telegram" {
		results = append(results, checkTelegramBot(cfg.Approval))
	}

	// f. Audit log directory writable
	results = append(results, checkAuditLogDir())

	// g. Config file permissions
	results = append(results, checkConfigFilePerms())

	printDoctorTable(results)
	return nil
}

func checkConfig() checkResult {
	_, err := config.Load()
	if err != nil {
		return checkResult{Name: "Config file", Status: statusFail, Detail: err.Error()}
	}
	return checkResult{Name: "Config file", Status: statusPass, Detail: "found and valid"}
}

func checkHostPing(cmd *cobra.Command, tr transport.Transport, hostName string) checkResult {
	result, err := tr.Ping(cmd.Context(), hostName)
	if err != nil {
		return checkResult{
			Name:   fmt.Sprintf("Host %s reachable", hostName),
			Status: statusFail,
			Detail: err.Error(),
		}
	}
	if !result.Online {
		return checkResult{
			Name:   fmt.Sprintf("Host %s reachable", hostName),
			Status: statusFail,
			Detail: "host unreachable",
		}
	}
	return checkResult{
		Name:   fmt.Sprintf("Host %s reachable", hostName),
		Status: statusPass,
		Detail: fmt.Sprintf("latency %s", result.Latency.Round(time.Millisecond)),
	}
}

func checkDockerInstalled(cmd *cobra.Command, tr transport.Transport, hostName string) checkResult {
	result, err := tr.Exec(cmd.Context(), hostName, "docker --version")
	if err != nil {
		return checkResult{
			Name:   fmt.Sprintf("Docker on %s", hostName),
			Status: statusFail,
			Detail: err.Error(),
		}
	}
	if result.ExitCode != 0 {
		return checkResult{
			Name:   fmt.Sprintf("Docker on %s", hostName),
			Status: statusFail,
			Detail: "docker not found or not executable",
		}
	}
	version := strings.TrimSpace(result.Stdout)
	if len(version) > 60 {
		version = version[:60]
	}
	return checkResult{
		Name:   fmt.Sprintf("Docker on %s", hostName),
		Status: statusPass,
		Detail: version,
	}
}

func checkDockerComposeInstalled(cmd *cobra.Command, tr transport.Transport, hostName string) checkResult {
	res, err := tr.Exec(cmd.Context(), hostName, "docker compose version --short")
	if err != nil {
		return checkResult{
			Name:   fmt.Sprintf("Docker Compose on %s", hostName),
			Status: statusWarn,
			Detail: "not installed",
		}
	}
	if res.ExitCode != 0 {
		return checkResult{
			Name:   fmt.Sprintf("Docker Compose on %s", hostName),
			Status: statusWarn,
			Detail: "not installed",
		}
	}
	version := strings.TrimSpace(res.Stdout)
	return checkResult{
		Name:   fmt.Sprintf("Docker Compose on %s", hostName),
		Status: statusPass,
		Detail: "v" + version,
	}
}

func checkSSHKeyPerms() checkResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return checkResult{Name: "SSH key permissions", Status: statusWarn, Detail: "cannot determine home directory"}
	}

	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		path := filepath.Join(home, ".ssh", name)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			continue
		}
		mode := info.Mode().Perm()
		if mode&0o044 != 0 {
			return checkResult{
				Name:   "SSH key permissions",
				Status: statusWarn,
				Detail: fmt.Sprintf("%s is world/group-readable (%04o) — run: chmod 600 %s", path, mode, path),
			}
		}
	}
	return checkResult{Name: "SSH key permissions", Status: statusPass, Detail: "key files have safe permissions"}
}

func checkTelegramBot(approval *config.ApprovalConfig) checkResult {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", approval.BotToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL) //nolint:noctx
	if err != nil {
		return checkResult{Name: "Telegram bot", Status: statusFail, Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return checkResult{Name: "Telegram bot", Status: statusFail, Detail: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return checkResult{Name: "Telegram bot", Status: statusPass, Detail: "bot reachable"}
}

func checkAuditLogDir() checkResult {
	dir := filepath.Dir(config.AuditLogPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return checkResult{Name: "Audit log dir", Status: statusFail, Detail: err.Error()}
	}
	tmpFile := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(tmpFile, []byte{}, 0o600); err != nil {
		return checkResult{Name: "Audit log dir", Status: statusFail, Detail: fmt.Sprintf("not writable: %s", err)}
	}
	os.Remove(tmpFile)
	return checkResult{Name: "Audit log dir", Status: statusPass, Detail: dir}
}

func checkConfigFilePerms() checkResult {
	for _, p := range config.DefaultConfigPaths() {
		info, err := os.Stat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			continue
		}
		mode := info.Mode().Perm()
		if mode&0o077 != 0 {
			return checkResult{
				Name:   "Config file permissions",
				Status: statusWarn,
				Detail: fmt.Sprintf("%s permissions are %04o — run: chmod 600 %s", p, mode, p),
			}
		}
		return checkResult{
			Name:   "Config file permissions",
			Status: statusPass,
			Detail: fmt.Sprintf("%s (%04o)", p, mode),
		}
	}
	return checkResult{Name: "Config file permissions", Status: statusWarn, Detail: "no config file found"}
}

func printDoctorTable(results []checkResult) {
	passColor := color.New(color.FgGreen, color.Bold)
	warnColor := color.New(color.FgYellow, color.Bold)
	failColor := color.New(color.FgRed, color.Bold)

	fmt.Printf("%-42s %-6s %s\n", "Check", "Status", "Detail")
	fmt.Println(strings.Repeat("-", 80))
	for _, r := range results {
		var statusStr string
		switch r.Status {
		case statusPass:
			statusStr = passColor.Sprint("PASS")
		case statusWarn:
			statusStr = warnColor.Sprint("WARN")
		case statusFail:
			statusStr = failColor.Sprint("FAIL")
		}
		fmt.Printf("%-42s %-15s %s\n", r.Name, statusStr, r.Detail)
	}
}
