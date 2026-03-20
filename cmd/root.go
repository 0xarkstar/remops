package cmd

import (
	"fmt"
	"os"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/output"
	"github.com/spf13/cobra"
)

// Exit codes.
const (
	ExitSuccess          = 0
	ExitGeneralError     = 1
	ExitPartialFailure   = 2
	ExitConfigError      = 3
	ExitConnectionError  = 4
	ExitPermissionDenied = 5
	ExitApprovalPending  = 6
	ExitRateLimited      = 7
)

var (
	flagFormat   string
	flagProfile  string
	flagHost     string
	flagTag      string
	flagVerbose  bool
	flagSanitize bool
	flagTimeout  string
	flagDryRun   bool

	cfg *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "remops",
	Short: "Agent-first CLI for remote Docker operations",
	Long: `remops — One CLI, all your servers. Built for AI agents. Designed for humans.

remops manages Docker containers across multiple remote hosts via SSH.
It provides structured output, permission levels, and out-of-band approval
for safe AI agent integration.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config loading for help, completion, version, and root
		if cmd.Name() == "help" || cmd.Name() == "completion" || cmd.Name() == "remops" {
			return nil
		}

		// Allow doctor to run with partial config
		if cmd.Name() == "doctor" {
			cfg, _ = config.Load()
			return nil
		}

		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("config error: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagFormat, "format", "f", "auto", "Output format: json, table, auto")
	rootCmd.PersistentFlags().StringVarP(&flagProfile, "profile", "p", "admin", "Permission profile to use")
	rootCmd.PersistentFlags().StringVar(&flagHost, "host", "", "Target host name")
	rootCmd.PersistentFlags().StringVar(&flagTag, "tag", "", "Filter by tag")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolVar(&flagSanitize, "sanitize", false, "Sanitize output (strip LLM directives)")
	rootCmd.PersistentFlags().StringVar(&flagTimeout, "timeout", "", "Override per-host timeout")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Show what would happen without executing")

	// Register dynamic completions after flags are defined.
	registerCompletions()
}

// Execute runs the root command.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

// getFormatter returns the appropriate output formatter.
func getFormatter() output.Formatter {
	return output.NewFormatter(output.Format(flagFormat))
}

// resolveHosts returns the list of host names to operate on.
func resolveHosts() []string {
	if cfg == nil {
		return nil
	}
	if flagHost != "" {
		if _, ok := cfg.Hosts[flagHost]; ok {
			return []string{flagHost}
		}
		return nil
	}
	if flagTag != "" {
		return cfg.HostsByTag(flagTag)
	}
	return cfg.AllHostNames()
}
