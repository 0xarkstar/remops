package alerting

import (
	"fmt"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/spf13/cobra"
)

// AlertingPlugin provides alert notification commands.
type AlertingPlugin struct {
	cfg *config.Config
}

// New creates a new AlertingPlugin.
func New() *AlertingPlugin {
	return &AlertingPlugin{}
}

func (p *AlertingPlugin) Name() string        { return "alerting" }
func (p *AlertingPlugin) Version() string     { return "0.1.0" }
func (p *AlertingPlugin) Description() string { return "Alert notifications via Telegram, Discord, Slack" }

func (p *AlertingPlugin) Init(cfg *config.Config) error {
	p.cfg = cfg
	return nil
}

func (p *AlertingPlugin) Commands() []*cobra.Command {
	alertCmd := &cobra.Command{
		Use:   "alert",
		Short: "Manage alert notifications",
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure alert channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Alert setup wizard (coming soon)")
			return nil
		},
	}

	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Send a test alert",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Sending test alert... (coming soon)")
			return nil
		},
	}

	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "Show alert history",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Alert history (coming soon)")
			return nil
		},
	}

	alertCmd.AddCommand(setupCmd, testCmd, historyCmd)
	return []*cobra.Command{alertCmd}
}
