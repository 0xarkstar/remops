package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/mcp"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP stdio server for Claude Code integration",
	Long: `Start an MCP (Model Context Protocol) stdio server.

Reads JSON-RPC 2.0 requests from stdin and writes responses to stdout.
All logging goes to stderr. Do not run interactively.`,
	// Override parent PersistentPreRunE so nothing is written to stdout
	// before the JSON-RPC loop begins.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "remops mcp: config error: %v\n", err)
			os.Exit(ExitConfigError)
		}

		t := transport.NewSSHTransport(cfg)
		defer t.Close()

		opts := []mcp.ServerOption{
			mcp.WithProfile(config.ParseLevel(flagProfile)),
			mcp.WithVersion(buildVersion),
		}

		if cfg.Approval != nil && cfg.Approval.Method == "telegram" {
			opts = append(opts, mcp.WithApprover(
				security.NewTelegramApprover(cfg.Approval.BotToken, cfg.Approval.ChatID),
			))
		}

		if cfg.Approval != nil && cfg.Approval.RateLimit != nil {
			rl, err := security.NewRateLimiter(cfg.Approval.RateLimit.EffectiveMaxWrites())
			if err != nil {
				fmt.Fprintf(os.Stderr, "remops mcp: rate limiter: %v\n", err)
			} else {
				opts = append(opts, mcp.WithRateLimiter(rl))
			}
		}

		al, err := security.NewAuditLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "remops mcp: audit logger: %v\n", err)
		} else {
			defer al.Close()
			opts = append(opts, mcp.WithAuditLogger(al))
		}

		server := mcp.NewServer(cfg, t, opts...)
		return server.Run(context.Background())
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
