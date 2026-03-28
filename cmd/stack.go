package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/docker"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"
	"github.com/spf13/cobra"
)

var stackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Manage Docker Compose stacks defined in config",
}

func init() {
	// stack ps
	psCmd := &cobra.Command{
		Use:   "ps <name>",
		Short: "Show status of a compose stack",
		Args:  cobra.ExactArgs(1),
		RunE:  runStackPS,
	}

	// stack logs
	logsCmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Show logs for a compose stack",
		Args:  cobra.ExactArgs(1),
		RunE:  runStackLogs,
	}
	logsCmd.Flags().IntVar(&flagServiceLogsTail, "tail", 100, "Number of log lines")
	logsCmd.Flags().StringVar(&flagServiceLogsSince, "since", "", "Since duration")
	logsCmd.Flags().String("service", "", "Filter to specific service within stack")

	// stack up
	upCmd := &cobra.Command{
		Use:   "up <name>",
		Short: "Start or update a compose stack",
		Args:  cobra.ExactArgs(1),
		RunE:  runStackWrite("up -d"),
	}
	upCmd.Flags().Bool("confirm", false, "Confirm execution")

	// stack pull
	pullCmd := &cobra.Command{
		Use:   "pull <name>",
		Short: "Pull new images for a compose stack",
		Args:  cobra.ExactArgs(1),
		RunE:  runStackWrite("pull"),
	}
	pullCmd.Flags().Bool("confirm", false, "Confirm execution")

	// stack restart
	restartCmd := &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a compose stack",
		Args:  cobra.ExactArgs(1),
		RunE:  runStackWrite("restart"),
	}
	restartCmd.Flags().Bool("confirm", false, "Confirm execution")

	// stack down (admin only)
	downCmd := &cobra.Command{
		Use:   "down <name>",
		Short: "Stop and remove a compose stack (admin only)",
		Args:  cobra.ExactArgs(1),
		RunE:  runStackDown,
	}
	downCmd.Flags().Bool("confirm", false, "Confirm execution")

	stackCmd.AddCommand(psCmd, logsCmd, upCmd, pullCmd, restartCmd, downCmd)
	rootCmd.AddCommand(stackCmd)
}

func resolveStack(name string) (config.Stack, error) {
	if cfg == nil {
		return config.Stack{}, fmt.Errorf("config not loaded")
	}
	stack, ok := cfg.Stacks[name]
	if !ok {
		available := cfg.AllStackNames()
		if len(available) > 0 {
			return config.Stack{}, fmt.Errorf("stack %q not found. Available: %s", name, strings.Join(available, ", "))
		}
		return config.Stack{}, fmt.Errorf("stack %q not found. No stacks defined in config", name)
	}
	return stack, nil
}

func runStackPS(cmd *cobra.Command, args []string) error {
	stack, err := resolveStack(args[0])
	if err != nil {
		return err
	}
	if err := security.CheckPermission(currentProfileLevel(), config.LevelViewer); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitPermissionDenied)
	}

	tr := transport.NewSSHTransport(cfg)
	defer tr.Close()
	dc := docker.NewDockerClient(tr)

	output, err := dc.ComposePS(cmd.Context(), stack.Host, stack.Path)
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

func runStackLogs(cmd *cobra.Command, args []string) error {
	stack, err := resolveStack(args[0])
	if err != nil {
		return err
	}
	if err := security.CheckPermission(currentProfileLevel(), config.LevelViewer); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitPermissionDenied)
	}
	if flagServiceLogsSince != "" {
		if err := security.DetectShellInjection(flagServiceLogsSince); err != nil {
			return fmt.Errorf("invalid --since: %w", err)
		}
	}
	svcFilter, _ := cmd.Flags().GetString("service")
	if svcFilter != "" {
		if err := security.ValidateServiceName(svcFilter); err != nil {
			return err
		}
	}

	tr := transport.NewSSHTransport(cfg)
	defer tr.Close()
	dc := docker.NewDockerClient(tr)

	output, err := dc.ComposeLogs(cmd.Context(), stack.Host, stack.Path, flagServiceLogsTail, flagServiceLogsSince, svcFilter)
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

func runStackWrite(action string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		stack, err := resolveStack(args[0])
		if err != nil {
			return err
		}
		if err := security.CheckPermission(currentProfileLevel(), config.LevelOperator); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(ExitPermissionDenied)
		}

		composeCmd := fmt.Sprintf("cd '%s' && docker compose %s", strings.ReplaceAll(stack.Path, "'", "'\\''"), action)

		confirm, _ := cmd.Flags().GetBool("confirm")
		if flagDryRun || !confirm {
			fmt.Printf("[dry-run] would execute on %s: %s\n", stack.Host, composeCmd)
			fmt.Println("Pass --confirm to execute.")
			return nil
		}

		tr := transport.NewSSHTransport(cfg)
		defer tr.Close()
		dc := docker.NewDockerClient(tr)

		output, exitCode, err := dc.ComposeAction(cmd.Context(), stack.Host, stack.Path, action)
		if err != nil {
			return fmt.Errorf("stack %s %s: %w", action, args[0], err)
		}
		if exitCode != 0 {
			return fmt.Errorf("stack %s %s: exit code %d\n%s", action, args[0], exitCode, output)
		}
		fmt.Printf("OK: compose %s on %s (%s)\n", action, args[0], stack.Host)
		if output != "" {
			fmt.Print(output)
		}
		return nil
	}
}

func runStackDown(cmd *cobra.Command, args []string) error {
	stack, err := resolveStack(args[0])
	if err != nil {
		return err
	}
	// down is destructive — requires admin
	if err := security.CheckPermission(currentProfileLevel(), config.LevelAdmin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitPermissionDenied)
	}

	confirm, _ := cmd.Flags().GetBool("confirm")
	if flagDryRun || !confirm {
		fmt.Printf("[dry-run] would execute on %s: cd '%s' && docker compose down\n", stack.Host, stack.Path)
		fmt.Println("Pass --confirm to execute. WARNING: this removes containers and networks.")
		return nil
	}

	tr := transport.NewSSHTransport(cfg)
	defer tr.Close()
	dc := docker.NewDockerClient(tr)

	output, exitCode, err := dc.ComposeAction(cmd.Context(), stack.Host, stack.Path, "down")
	if err != nil {
		return fmt.Errorf("stack down %s: %w", args[0], err)
	}
	if exitCode != 0 {
		return fmt.Errorf("stack down %s: exit code %d\n%s", args[0], exitCode, output)
	}
	fmt.Printf("OK: compose down on %s (%s)\n", args[0], stack.Host)
	return nil
}
