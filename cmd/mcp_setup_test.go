package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupMCPConfig_NewFile(t *testing.T) {
	dir := t.TempDir()
	claudeJSON := filepath.Join(dir, ".claude.json")

	t.Setenv("HOME", dir)

	if err := setupMCPConfig(); err != nil {
		t.Fatalf("setupMCPConfig() error: %v", err)
	}

	raw, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	servers, ok := data["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers not found or wrong type")
	}

	remops, ok := servers["remops"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers.remops not found")
	}

	if remops["type"] != "stdio" {
		t.Errorf("type = %v, want stdio", remops["type"])
	}
	if remops["command"] == "" {
		t.Error("command should not be empty")
	}
}

func TestSetupMCPConfig_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	claudeJSON := filepath.Join(dir, ".claude.json")

	initial := map[string]any{
		"mcpServers": map[string]any{
			"other-tool": map[string]any{
				"command": "/usr/bin/other",
				"type":    "stdio",
			},
		},
		"someOtherField": "preserved",
	}
	raw, _ := json.Marshal(initial)
	if err := os.WriteFile(claudeJSON, raw, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("HOME", dir)

	if err := setupMCPConfig(); err != nil {
		t.Fatalf("setupMCPConfig() error: %v", err)
	}

	out, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(out, &data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Original field preserved.
	if data["someOtherField"] != "preserved" {
		t.Errorf("someOtherField = %v, want preserved", data["someOtherField"])
	}

	servers, ok := data["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers not found")
	}

	// Pre-existing server preserved.
	if _, ok := servers["other-tool"]; !ok {
		t.Error("other-tool entry should be preserved")
	}

	// remops added.
	if _, ok := servers["remops"]; !ok {
		t.Error("remops entry should be added")
	}
}

func TestSetupMCPConfig_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	claudeJSON := filepath.Join(dir, ".claude.json")

	initial := map[string]any{
		"mcpServers": map[string]any{
			"remops": map[string]any{
				"command": "/old/path/remops",
				"args":    []string{"mcp"},
				"type":    "stdio",
			},
		},
	}
	raw, _ := json.Marshal(initial)
	if err := os.WriteFile(claudeJSON, raw, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("HOME", dir)

	if err := setupMCPConfig(); err != nil {
		t.Fatalf("setupMCPConfig() error: %v", err)
	}

	out, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(out, &data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	servers := data["mcpServers"].(map[string]any)
	remops := servers["remops"].(map[string]any)

	// Should be updated, not the old path.
	if remops["command"] == "/old/path/remops" {
		t.Error("command should have been updated from old path")
	}

	// Args should include the profile flag.
	args, ok := remops["args"].([]any)
	if !ok {
		t.Fatal("args not found or wrong type")
	}
	if len(args) != 3 {
		t.Errorf("args len = %d, want 3", len(args))
	}
	if args[1] != "--profile" {
		t.Errorf("args[1] = %v, want --profile", args[1])
	}
}
