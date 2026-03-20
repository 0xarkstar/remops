package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// writeTestConfig writes a remops.yaml to a temp dir and sets REMOPS_CONFIG.
// The caller must restore REMOPS_CONFIG via t.Cleanup.
func writeTestConfig(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "remops.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTestConfig: %v", err)
	}
	orig := os.Getenv("REMOPS_CONFIG")
	t.Setenv("REMOPS_CONFIG", p)
	t.Cleanup(func() { os.Setenv("REMOPS_CONFIG", orig) })
}

const testConfigYAML = `
version: 1
hosts:
  prod:
    address: 1.2.3.4
    tags: [production, web]
  dev:
    address: 5.6.7.8
    tags: [staging]
services:
  app:
    host: prod
    container: app_container
    tags: [backend]
profiles:
  viewer:
    level: viewer
  operator:
    level: operator
`

func TestCompleteHostNames_AllHosts(t *testing.T) {
	writeTestConfig(t, testConfigYAML)

	names, directive := completeHostNames(&cobra.Command{}, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}
	if len(names) != 2 {
		t.Errorf("completeHostNames('') = %v, want 2 names", names)
	}
}

func TestCompleteHostNames_Prefix(t *testing.T) {
	writeTestConfig(t, testConfigYAML)

	names, _ := completeHostNames(&cobra.Command{}, nil, "pr")
	if len(names) != 1 || names[0] != "prod" {
		t.Errorf("completeHostNames('pr') = %v, want [prod]", names)
	}
}

func TestCompleteHostNames_NoMatch(t *testing.T) {
	writeTestConfig(t, testConfigYAML)

	names, _ := completeHostNames(&cobra.Command{}, nil, "xyz")
	if len(names) != 0 {
		t.Errorf("completeHostNames('xyz') = %v, want empty", names)
	}
}

func TestCompleteProfileNames_All(t *testing.T) {
	writeTestConfig(t, testConfigYAML)

	names, directive := completeProfileNames(&cobra.Command{}, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}
	if len(names) != 2 {
		t.Errorf("completeProfileNames('') = %v, want 2 names", names)
	}
}

func TestCompleteProfileNames_Prefix(t *testing.T) {
	writeTestConfig(t, testConfigYAML)

	names, _ := completeProfileNames(&cobra.Command{}, nil, "view")
	if len(names) != 1 || names[0] != "viewer" {
		t.Errorf("completeProfileNames('view') = %v, want [viewer]", names)
	}
}

func TestCompleteTags_All(t *testing.T) {
	writeTestConfig(t, testConfigYAML)

	tags, directive := completeTags(&cobra.Command{}, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}
	// Tags: production, web, staging, backend = 4 unique
	if len(tags) != 4 {
		t.Errorf("completeTags('') = %v, want 4 tags", tags)
	}
}

func TestCompleteTags_Prefix(t *testing.T) {
	writeTestConfig(t, testConfigYAML)

	tags, _ := completeTags(&cobra.Command{}, nil, "pro")
	if len(tags) != 1 || tags[0] != "production" {
		t.Errorf("completeTags('pro') = %v, want [production]", tags)
	}
}

func TestCompleteServiceNames_All(t *testing.T) {
	writeTestConfig(t, testConfigYAML)

	names, directive := completeServiceNames(&cobra.Command{}, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}
	if len(names) != 1 || names[0] != "app" {
		t.Errorf("completeServiceNames('') = %v, want [app]", names)
	}
}

func TestCompleteServiceNames_Prefix(t *testing.T) {
	writeTestConfig(t, testConfigYAML)

	names, _ := completeServiceNames(&cobra.Command{}, nil, "ap")
	if len(names) != 1 {
		t.Errorf("completeServiceNames('ap') = %v, want [app]", names)
	}
}

func TestCompleteSinceDurations_All(t *testing.T) {
	durations, directive := completeSinceDurations(&cobra.Command{}, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}
	if len(durations) == 0 {
		t.Error("completeSinceDurations('') returned no suggestions")
	}
}

func TestCompleteSinceDurations_Prefix(t *testing.T) {
	durations, _ := completeSinceDurations(&cobra.Command{}, nil, "1")
	for _, d := range durations {
		if len(d) == 0 || d[0] != '1' {
			t.Errorf("completeSinceDurations('1') returned non-matching %q", d)
		}
	}
	if len(durations) == 0 {
		t.Error("expected at least one match for prefix '1'")
	}
}

func TestCompletionCommand_Bash(t *testing.T) {
	// Capture stdout since completion writes directly to os.Stdout, not cobra's writer.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	rootCmd.SetArgs([]string{"completion", "bash"})
	execErr := rootCmd.Execute()

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)

	if execErr != nil {
		t.Fatalf("completion bash returned error: %v", execErr)
	}
	if buf.Len() == 0 {
		t.Error("completion bash produced empty output")
	}
}

func TestCompleteHostNames_NilConfig(t *testing.T) {
	// Point REMOPS_CONFIG to a non-existent path so loadConfigSilent returns nil
	orig := os.Getenv("REMOPS_CONFIG")
	t.Setenv("REMOPS_CONFIG", "/nonexistent/path/remops.yaml")
	t.Cleanup(func() { os.Setenv("REMOPS_CONFIG", orig) })
	// Also clear XDG and cwd lookup
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Cleanup(func() { os.Setenv("XDG_CONFIG_HOME", origXDG) })

	names, directive := completeHostNames(&cobra.Command{}, nil, "")
	if names != nil {
		t.Errorf("expected nil names with no config, got %v", names)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}
}
