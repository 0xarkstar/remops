package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"
	"github.com/spf13/cobra"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage services defined in config",
}

var (
	flagServiceLogsTail   int
	flagServiceLogsSince  string
	flagServiceLogsFollow bool
)

func init() {
	logsCmd := &cobra.Command{
		Use:               "logs <name>",
		Short:             "Fetch logs for a service",
		Annotations:       map[string]string{"permission": "viewer"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeServiceNames,
		RunE:              runServiceLogs,
	}
	logsCmd.Flags().IntVar(&flagServiceLogsTail, "tail", 100, "Number of log lines to show")
	logsCmd.Flags().StringVar(&flagServiceLogsSince, "since", "", "Show logs since duration or timestamp (e.g. 1h, 2024-01-01T00:00:00)")
	logsCmd.Flags().BoolVar(&flagServiceLogsFollow, "follow", false, "Follow log output")

	restartCmd := &cobra.Command{
		Use:               "restart <name>",
		Short:             "Restart a service",
		Annotations:       map[string]string{"permission": "operator"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeServiceNames,
		RunE:              runServiceWrite("restart"),
	}
	restartCmd.Flags().Bool("confirm", false, "Confirm execution (omit to see dry-run)")

	stopCmd := &cobra.Command{
		Use:               "stop <name>",
		Short:             "Stop a service",
		Annotations:       map[string]string{"permission": "operator"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeServiceNames,
		RunE:              runServiceWrite("stop"),
	}
	stopCmd.Flags().Bool("confirm", false, "Confirm execution (omit to see dry-run)")

	startCmd := &cobra.Command{
		Use:               "start <name>",
		Short:             "Start a service",
		Annotations:       map[string]string{"permission": "operator"},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeServiceNames,
		RunE:              runServiceWrite("start"),
	}
	startCmd.Flags().Bool("confirm", false, "Confirm execution (omit to see dry-run)")

	serviceCmd.AddCommand(logsCmd, restartCmd, stopCmd, startCmd)
	rootCmd.AddCommand(serviceCmd)
}

// resolveService looks up a service by name and returns it.
func resolveService(name string) (config.Service, error) {
	if cfg == nil {
		return config.Service{}, fmt.Errorf("config not loaded")
	}
	svc, ok := cfg.Services[name]
	if !ok {
		available := cfg.AllServiceNames()
		if len(available) > 0 {
			return config.Service{}, fmt.Errorf("service %q not found. Available services: %s", name, strings.Join(available, ", "))
		}
		return config.Service{}, fmt.Errorf("service %q not found. No services defined in config", name)
	}
	return svc, nil
}

// currentProfileLevel returns the PermissionLevel for the active flagProfile.
func currentProfileLevel() config.PermissionLevel {
	if cfg == nil {
		return config.LevelAdmin
	}
	profile, ok := cfg.Profiles[flagProfile]
	if !ok {
		if flagProfile != "admin" {
			fmt.Fprintf(os.Stderr, "warning: profile %q not found in config, defaulting to admin\n", flagProfile)
		}
		return config.LevelAdmin
	}
	return config.ParseLevel(profile.Level)
}

func runServiceLogs(cmd *cobra.Command, args []string) error {
	svc, err := resolveService(args[0])
	if err != nil {
		return err
	}

	if err := security.CheckPermission(currentProfileLevel(), config.LevelViewer); err != nil {
		fmt.Fprintf(os.Stderr, "%v\nHint: use --profile operator or --profile admin\n", err)
		os.Exit(ExitPermissionDenied)
	}

	if flagServiceLogsSince != "" {
		if err := security.DetectShellInjection(flagServiceLogsSince); err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
	}

	tr := transport.NewSSHTransport(cfg)
	defer tr.Close()

	dockerCmd := buildLogsCmd(svc.Container)
	ctx := cmd.Context()

	if flagServiceLogsFollow {
		reader, err := tr.Stream(ctx, svc.Host, dockerCmd)
		if err != nil {
			return fmt.Errorf("stream logs for %s: %w", args[0], err)
		}
		defer reader.Close()
		_, err = io.Copy(os.Stdout, reader)
		return err
	}

	result, err := tr.Exec(ctx, svc.Host, dockerCmd)
	if err != nil {
		return fmt.Errorf("exec logs for %s: %w", args[0], err)
	}
	fmt.Print(result.Stdout)
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}
	return nil
}

func buildLogsCmd(container string) string {
	parts := []string{"docker", "logs", container, "--tail", strconv.Itoa(flagServiceLogsTail)}
	if flagServiceLogsSince != "" {
		parts = append(parts, "--since", flagServiceLogsSince)
	}
	return strings.Join(parts, " ")
}

func runServiceWrite(action string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		svc, err := resolveService(args[0])
		if err != nil {
			return err
		}

		if err := security.CheckPermission(currentProfileLevel(), config.LevelOperator); err != nil {
			fmt.Fprintf(os.Stderr, "%v\nHint: use --profile operator or --profile admin\n", err)
			os.Exit(ExitPermissionDenied)
		}

		dockerCmd := fmt.Sprintf("docker %s %s", action, svc.Container)

		confirm, _ := cmd.Flags().GetBool("confirm")
		if flagDryRun || !confirm {
			fmt.Printf("[dry-run] would execute on %s: %s\n", svc.Host, dockerCmd)
			fmt.Println("Pass --confirm to execute this operation.")
			return nil
		}

		tr := transport.NewSSHTransport(cfg)
		defer tr.Close()

		result, err := tr.Exec(cmd.Context(), svc.Host, dockerCmd)
		if err != nil {
			return fmt.Errorf("%s service %s: %w", action, args[0], err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("%s service %s: docker exited %d: %s", action, args[0], result.ExitCode, strings.TrimSpace(result.Stderr))
		}
		fmt.Printf("OK: %s %s on %s\n", action, args[0], svc.Host)
		return nil
	}
}
