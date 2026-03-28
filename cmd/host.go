package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/docker"
	"github.com/0xarkstar/remops/internal/output"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"
	"github.com/spf13/cobra"
)

var hostCmd = &cobra.Command{
	Use:   "host",
	Short: "Host management commands",
}

var hostInfoCmd = &cobra.Command{
	Use:               "info <name>",
	Short:             "Show system info and containers for a host",
	Annotations:       map[string]string{"permission": "viewer"},
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeHostNames,
	RunE:              runHostInfo,
}

func init() {
	diskCmd := &cobra.Command{
		Use:               "disk [name]",
		Short:             "Show disk usage for host(s)",
		Annotations:       map[string]string{"permission": "viewer"},
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeHostNames,
		RunE:              runHostDisk,
	}

	pruneCmd := &cobra.Command{
		Use:               "prune <name>",
		Short:             "Prune unused Docker data on a host",
		Annotations:       map[string]string{"permission": "operator"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeHostNames,
		RunE:              runHostPrune,
	}
	pruneCmd.Flags().Bool("confirm", false, "Confirm execution")
	pruneCmd.Flags().Bool("volumes", false, "Also prune unused volumes")

	execCmd := &cobra.Command{
		Use:               `exec <name> "<command>"`,
		Short:             "Execute an arbitrary command on a host (admin only)",
		Annotations:       map[string]string{"permission": "admin"},
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeHostNames,
		RunE:              runHostExec,
	}
	execCmd.Flags().Bool("confirm", false, "Confirm execution")

	hostCmd.AddCommand(hostInfoCmd, diskCmd, pruneCmd, execCmd)
	rootCmd.AddCommand(hostCmd)
}

func runHostInfo(cmd *cobra.Command, args []string) error {
	hostName := args[0]
	if _, ok := cfg.Hosts[hostName]; !ok {
		available := cfg.AllHostNames()
		if len(available) > 0 {
			return fmt.Errorf("unknown host %q. Available hosts: %s", hostName, strings.Join(available, ", "))
		}
		return fmt.Errorf("unknown host %q. No hosts defined in config", hostName)
	}

	start := time.Now()
	t := transport.NewSSHTransport(cfg)
	defer t.Close()

	dc := docker.NewDockerClient(t)
	ctx := cmd.Context()

	resp := output.NewResponse()
	result, err := gatherHostData(ctx, dc, hostName)
	if err != nil {
		resp.AddFailure(hostName, "exec_error", err.Error(), "check SSH connectivity and Docker installation")
	} else {
		resp.AddResult(result)
	}

	resp.Finalize(start)
	return getFormatter().Format(os.Stdout, resp)
}

func runHostDisk(cmd *cobra.Command, args []string) error {
	if err := security.CheckPermission(currentProfileLevel(), config.LevelViewer); err != nil {
		fmt.Fprintf(os.Stderr, "%v\nHint: use --profile operator or --profile admin\n", err)
		os.Exit(ExitPermissionDenied)
	}

	var hosts []string
	if len(args) == 1 {
		name := args[0]
		if _, ok := cfg.Hosts[name]; !ok {
			available := cfg.AllHostNames()
			if len(available) > 0 {
				return fmt.Errorf("unknown host %q. Available hosts: %s", name, strings.Join(available, ", "))
			}
			return fmt.Errorf("unknown host %q. No hosts defined in config", name)
		}
		hosts = []string{name}
	} else {
		hosts = resolveHosts()
	}
	if len(hosts) == 0 {
		return fmt.Errorf("no hosts to query")
	}

	start := time.Now()
	tr := transport.NewSSHTransport(cfg)
	defer tr.Close()

	resp := output.NewResponse()
	ctx := cmd.Context()

	for _, h := range hosts {
		res, err := tr.Exec(ctx, h, "df -h --output=target,size,used,avail,pcent")
		if err != nil {
			resp.AddFailure(h, "exec_error", err.Error(), "check SSH connectivity")
			continue
		}
		resp.AddResult(map[string]any{
			"host":   h,
			"output": strings.TrimRight(res.Stdout, "\n"),
		})
	}

	resp.Finalize(start)
	return getFormatter().Format(os.Stdout, resp)
}

func runHostPrune(cmd *cobra.Command, args []string) error {
	hostName := args[0]
	if _, ok := cfg.Hosts[hostName]; !ok {
		available := cfg.AllHostNames()
		if len(available) > 0 {
			return fmt.Errorf("unknown host %q. Available hosts: %s", hostName, strings.Join(available, ", "))
		}
		return fmt.Errorf("unknown host %q. No hosts defined in config", hostName)
	}

	if err := security.CheckPermission(currentProfileLevel(), config.LevelOperator); err != nil {
		fmt.Fprintf(os.Stderr, "%v\nHint: use --profile operator or --profile admin\n", err)
		os.Exit(ExitPermissionDenied)
	}

	tr := transport.NewSSHTransport(cfg)
	defer tr.Close()

	ctx := cmd.Context()

	if flagDryRun {
		res, err := tr.Exec(ctx, hostName, "docker system df")
		if err != nil {
			return fmt.Errorf("docker system df on %s: %w", hostName, err)
		}
		fmt.Printf("[dry-run] docker system df on %s:\n%s\n", hostName, strings.TrimRight(res.Stdout, "\n"))
		fmt.Println("Pass --confirm to prune (removes unused data).")
		return nil
	}

	confirm, _ := cmd.Flags().GetBool("confirm")
	if !confirm {
		fmt.Printf("[dry-run] would execute docker system prune -f on %s\n", hostName)
		fmt.Println("Pass --confirm to execute this operation.")
		return nil
	}

	volumes, _ := cmd.Flags().GetBool("volumes")
	dockerCmd := "docker system prune -f"
	if volumes {
		dockerCmd += " --volumes"
	}

	res, err := tr.Exec(ctx, hostName, dockerCmd)
	if err != nil {
		return fmt.Errorf("prune %s: %w", hostName, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("prune %s: docker exited %d: %s", hostName, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	fmt.Print(res.Stdout)
	return nil
}

func runHostExec(cmd *cobra.Command, args []string) error {
	hostName := args[0]
	command := args[1]

	if _, ok := cfg.Hosts[hostName]; !ok {
		available := cfg.AllHostNames()
		if len(available) > 0 {
			return fmt.Errorf("unknown host %q. Available hosts: %s", hostName, strings.Join(available, ", "))
		}
		return fmt.Errorf("unknown host %q. No hosts defined in config", hostName)
	}

	if err := security.CheckPermission(currentProfileLevel(), config.LevelAdmin); err != nil {
		fmt.Fprintf(os.Stderr, "%v\nHint: use --profile admin\n", err)
		os.Exit(ExitPermissionDenied)
	}

	confirm, _ := cmd.Flags().GetBool("confirm")
	if flagDryRun || !confirm {
		fmt.Printf("[dry-run] would execute on %s: %s\n", hostName, command)
		fmt.Println("Pass --confirm to execute this operation.")
		return nil
	}

	al, err := security.NewAuditLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open audit log: %v\n", err)
	} else {
		defer al.Close()
		_ = al.Log(security.AuditEntry{
			Command: command,
			Host:    hostName,
			Profile: flagProfile,
			Result:  "exec",
		})
	}

	tr := transport.NewSSHTransport(cfg)
	defer tr.Close()

	res, err := tr.Exec(cmd.Context(), hostName, command)
	if err != nil {
		return fmt.Errorf("exec on %s: %w", hostName, err)
	}
	fmt.Print(res.Stdout)
	if res.Stderr != "" {
		fmt.Fprint(os.Stderr, res.Stderr)
	}
	return nil
}
