package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/output"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"
	"github.com/spf13/cobra"
)

// securityFinding is a single check result.
type securityFinding struct {
	Severity string `json:"severity"` // INFO or WARN
	Check    string `json:"check"`
	Message  string `json:"message"`
}

var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "Security scanning commands",
}

func init() {
	scanCmd := &cobra.Command{
		Use:         "scan [name]",
		Short:       "Run security checks on host(s)",
		Annotations: map[string]string{"permission": "viewer"},
		Args:        cobra.MaximumNArgs(1),
		RunE:        runSecurityScan,
	}
	securityCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(securityCmd)
}

func runSecurityScan(cmd *cobra.Command, args []string) error {
	if err := security.CheckPermission(currentProfileLevel(), config.LevelViewer); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitPermissionDenied)
	}

	var hosts []string
	if len(args) == 1 {
		name := args[0]
		if _, ok := cfg.Hosts[name]; !ok {
			return fmt.Errorf("unknown host %q", name)
		}
		hosts = []string{name}
	} else {
		hosts = resolveHosts()
	}
	if len(hosts) == 0 {
		return fmt.Errorf("no hosts to scan")
	}

	start := time.Now()
	tr := transport.NewSSHTransport(cfg)
	defer tr.Close()

	resp := output.NewResponse()
	ctx := cmd.Context()

	for _, h := range hosts {
		var findings []securityFinding

		// Check 1: count running containers
		if res, err := tr.Exec(ctx, h, "docker ps --filter status=running -q | wc -l"); err == nil {
			count := strings.TrimSpace(res.Stdout)
			findings = append(findings, securityFinding{
				Severity: "INFO",
				Check:    "running_containers",
				Message:  fmt.Sprintf("%s containers running", count),
			})
		}

		// Check 2: publicly bound ports (0.0.0.0 binds are externally accessible)
		if res, err := tr.Exec(ctx, h, "docker ps --format '{{.Ports}}'"); err == nil {
			public := parsePorts(res.Stdout)
			if len(public) > 0 {
				findings = append(findings, securityFinding{
					Severity: "WARN",
					Check:    "public_ports",
					Message:  fmt.Sprintf("%d container(s) with publicly bound ports: %s", len(public), strings.Join(public, "; ")),
				})
			} else {
				findings = append(findings, securityFinding{
					Severity: "INFO",
					Check:    "public_ports",
					Message:  "no publicly bound ports found",
				})
			}
		}

		// Check 3: images using :latest tag (unpinned versions)
		if res, err := tr.Exec(ctx, h, "docker images --format '{{.Repository}}:{{.Tag}}' | grep ':latest'"); err == nil {
			latest := parseLatestImages(res.Stdout)
			if len(latest) > 0 {
				findings = append(findings, securityFinding{
					Severity: "WARN",
					Check:    "latest_tags",
					Message:  fmt.Sprintf("%d image(s) using :latest tag: %s", len(latest), strings.Join(latest, ", ")),
				})
			} else {
				findings = append(findings, securityFinding{
					Severity: "INFO",
					Check:    "latest_tags",
					Message:  "no images using :latest tag",
				})
			}
		}

		// Check 4: stale containers (running >30 days without restart)
		if res, err := tr.Exec(ctx, h, "docker ps --format '{{.Names}} {{.Status}}'"); err == nil {
			stale := parseStaleContainers(res.Stdout)
			if len(stale) > 0 {
				findings = append(findings, securityFinding{
					Severity: "WARN",
					Check:    "stale_containers",
					Message:  fmt.Sprintf("%d container(s) running >30 days without restart: %s", len(stale), strings.Join(stale, ", ")),
				})
			} else {
				findings = append(findings, securityFinding{
					Severity: "INFO",
					Check:    "stale_containers",
					Message:  "no stale containers found",
				})
			}
		}

		resp.AddResult(map[string]any{
			"host":     h,
			"findings": findingsToMaps(findings),
		})
	}

	resp.Finalize(start)
	return getFormatter().Format(os.Stdout, resp)
}

// parsePorts returns port strings that are publicly bound (0.0.0.0).
func parsePorts(out string) []string {
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && strings.Contains(line, "0.0.0.0:") {
			result = append(result, line)
		}
	}
	return result
}

// parseLatestImages returns image names that use the :latest tag.
func parseLatestImages(out string) []string {
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

// parseStaleContainers returns container names whose status indicates >30 days uptime.
func parseStaleContainers(out string) []string {
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && isStaleStatus(strings.Join(fields[1:], " ")) {
			result = append(result, fields[0])
		}
	}
	return result
}

// isStaleStatus returns true if a docker ps Status field indicates >30 days uptime.
func isStaleStatus(status string) bool {
	lower := strings.ToLower(status)
	return strings.Contains(lower, "up") && strings.Contains(lower, "month")
}

func findingsToMaps(findings []securityFinding) []map[string]any {
	result := make([]map[string]any, len(findings))
	for i, f := range findings {
		result[i] = map[string]any{
			"severity": f.Severity,
			"check":    f.Check,
			"message":  f.Message,
		}
	}
	return result
}
