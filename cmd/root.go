package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/output"
	"github.com/0xarkstar/remops/internal/plugin"
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
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

// SetVersionInfo sets the build version info from main.go ldflags.
func SetVersionInfo(version, commit, date string) {
	buildVersion = version
	buildCommit = commit
	buildDate = date
	rootCmd.Version = version
}

var (
	flagFormat   string
	flagProfile  string
	flagHost     string
	flagTag      string
	flagVerbose  bool
	flagSanitize bool
	flagTimeout  string
	flagDryRun   bool

	cfg            *config.Config
	pluginRegistry = plugin.NewRegistry()
)

var rootCmd = &cobra.Command{
	Use:   "remops",
	Short: "Agent-first CLI for remote Docker operations",
	Long: `remops — One CLI, all your servers. Built for AI agents. Designed for humans.

remops manages Docker containers and Compose stacks across multiple remote
hosts via SSH. It provides three interfaces (CLI, MCP, HTTP API) sharing
one security pipeline with permission levels and Telegram approval.

Getting started:
  remops init              Create config interactively
  remops init --mcp        Configure Claude Code MCP integration
  remops discover          Find containers on your hosts
  remops doctor            Verify connectivity
  remops status            See all containers across all hosts`,
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
			return fmt.Errorf("config error: %w\n\nRun 'remops init' to create a config file", err)
		}
		if len(cfg.Plugins) > 0 {
			if err := pluginRegistry.InitAll(cfg, cfg.Plugins); err != nil {
				fmt.Fprintf(os.Stderr, "plugin init warning: %v\n", err)
			}
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
		available := cfg.AllHostNames()
		fmt.Fprintf(os.Stderr, "host %q not found. Available hosts: %s\n", flagHost, strings.Join(available, ", "))
		return nil
	}
	if flagTag != "" {
		return cfg.HostsByTag(flagTag)
	}
	return cfg.AllHostNames()
}
