package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var mcpSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure Claude Code MCP integration",
	Long:  "Adds remops as an MCP server in ~/.claude.json for Claude Code integration.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil // skip config loading
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return setupMCPConfig()
	},
}

func init() {
	mcpCmd.AddCommand(mcpSetupCmd)
}

// setupMCPConfig adds or updates the remops entry in ~/.claude.json.
func setupMCPConfig() error {
	binaryPath, err := os.Executable()
	if err != nil {
		binaryPath = "remops"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	claudeJSON := filepath.Join(home, ".claude.json")

	data := map[string]any{}

	raw, err := os.ReadFile(claudeJSON)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", claudeJSON, err)
	}
	if err == nil {
		if jsonErr := json.Unmarshal(raw, &data); jsonErr != nil {
			return fmt.Errorf("failed to parse %s: %w", claudeJSON, jsonErr)
		}
	}

	// Ensure mcpServers map exists.
	mcpServers, _ := data["mcpServers"].(map[string]any)
	if mcpServers == nil {
		mcpServers = map[string]any{}
	}

	mcpServers["remops"] = map[string]any{
		"command": binaryPath,
		"args":    []string{"mcp", "--profile", "operator"},
		"type":    "stdio",
	}
	data["mcpServers"] = mcpServers

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(claudeJSON, out, 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", claudeJSON, err)
	}

	fmt.Println("Claude Code MCP integration configured. Restart Claude Code to activate.")
	return nil
}
