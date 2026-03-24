package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xarkstar/remops/internal/api"
	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"
	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Start HTTP API server for external agent integration",
	Long: `Start an HTTP API server that exposes remops operations over REST.

Enables integration with AI agents (Hermes, OpenClaw), webhooks,
and any HTTP client. All operations go through the same security
pipeline as CLI and MCP (permissions, approval, rate limiting, audit).

Requires api.api_key in config.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg == nil {
			return fmt.Errorf("config not loaded")
		}
		if cfg.API == nil {
			return fmt.Errorf("api section not configured in remops.yaml")
		}

		t := transport.NewSSHTransport(cfg)
		defer t.Close()

		opts := []api.ServerOption{
			api.WithProfile(config.ParseLevel(flagProfile)),
			api.WithVersion(buildVersion),
		}

		if cfg.Approval != nil && cfg.Approval.Method == "telegram" {
			opts = append(opts, api.WithApprover(
				security.NewTelegramApprover(cfg.Approval.BotToken, cfg.Approval.ChatID),
			))
		}

		if cfg.Approval != nil && cfg.Approval.RateLimit != nil {
			rl, err := security.NewRateLimiter(cfg.Approval.RateLimit.EffectiveMaxWrites())
			if err != nil {
				fmt.Fprintf(os.Stderr, "remops api: rate limiter: %v\n", err)
			} else {
				opts = append(opts, api.WithRateLimiter(rl))
			}
		}

		al, err := security.NewAuditLogger()
		if err != nil {
			fmt.Fprintf(os.Stderr, "remops api: audit logger: %v\n", err)
		} else {
			defer al.Close()
			opts = append(opts, api.WithAuditLogger(al))
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()

		server := api.NewServer(cfg, t, opts...)
		fmt.Fprintf(os.Stderr, "remops api: listening on %s\n", cfg.API.EffectiveListen())
		return server.Run(ctx, cfg.API.EffectiveListen())
	},
}

func init() {
	rootCmd.AddCommand(apiCmd)
}
