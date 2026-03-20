package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// resetRootCmd resets cobra state between tests to avoid flag re-definition
// and leftover args from previous test runs.
func resetRootCmd(t *testing.T) {
	t.Helper()
	rootCmd.ResetFlags()
	// Re-register persistent flags that init() would have set.
	rootCmd.PersistentFlags().StringVarP(&flagFormat, "format", "f", "auto", "Output format: json, table, auto")
	rootCmd.PersistentFlags().StringVarP(&flagProfile, "profile", "p", "admin", "Permission profile to use")
	rootCmd.PersistentFlags().StringVar(&flagHost, "host", "", "Target host name")
	rootCmd.PersistentFlags().StringVar(&flagTag, "tag", "", "Filter by tag")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolVar(&flagSanitize, "sanitize", false, "Sanitize output (strip LLM directives)")
	rootCmd.PersistentFlags().StringVar(&flagTimeout, "timeout", "", "Override per-host timeout")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Show what would happen without executing")
}

// captureStdout replaces os.Stdout with a pipe and returns the captured output
// after fn returns. Restores os.Stdout on cleanup.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	fn()

	w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("failed to read captured output: %v", err)
	}
	return buf.String()
}

func TestSmoke_VersionCommand(t *testing.T) {
	oldVersion := buildVersion
	oldCommit := buildCommit
	oldDate := buildDate
	t.Cleanup(func() {
		buildVersion = oldVersion
		buildCommit = oldCommit
		buildDate = oldDate
	})

	buildVersion = "0.0.0-test"
	buildCommit = "abc1234"
	buildDate = "2026-01-01"

	rootCmd.SetArgs([]string{"version"})

	var execErr error
	out := captureStdout(t, func() {
		execErr = rootCmd.Execute()
	})

	if execErr != nil {
		t.Fatalf("version command returned error: %v", execErr)
	}
	if !strings.Contains(out, "0.0.0-test") {
		t.Errorf("expected version string in output, got: %q", out)
	}
	if !strings.Contains(out, "remops") {
		t.Errorf("expected 'remops' in output, got: %q", out)
	}
}

func TestSmoke_HelpCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("root command (no args) returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "remops") {
		t.Errorf("expected 'remops' in help output, got: %q", out)
	}
}

func TestSmoke_HelpFlag(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--help"})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("--help returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "remops") {
		t.Errorf("expected 'remops' in --help output, got: %q", out)
	}
	if !strings.Contains(out, "Usage") {
		t.Errorf("expected 'Usage' in --help output, got: %q", out)
	}
}

func TestSmoke_InitCommand(t *testing.T) {
	// init reads os.Stdin directly, so we need to replace it.
	// Provide enough input to complete one host and decline adding another.
	// Prompts in order: host name, address, SSH user, SSH port, description, add another?
	input := "testhost\n192.0.2.1\ntestuser\n22\ntest description\nn\n"

	// Write to a temp file and redirect os.Stdin.
	tmpFile, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("failed to create temp stdin file: %v", err)
	}
	defer tmpFile.Close()
	if _, err := tmpFile.WriteString(input); err != nil {
		t.Fatalf("failed to write stdin data: %v", err)
	}
	if _, err := tmpFile.Seek(0, 0); err != nil {
		t.Fatalf("failed to seek stdin file: %v", err)
	}

	oldStdin := os.Stdin
	os.Stdin = tmpFile
	t.Cleanup(func() { os.Stdin = oldStdin })

	// Write config to a temp path so we don't touch the real config.
	tmpDir := t.TempDir()
	outputPath := tmpDir + "/remops.yaml"

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--output", outputPath})
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	err = rootCmd.Execute()
	if err != nil {
		t.Fatalf("init command returned error: %v", err)
	}

	// Verify config file was written.
	if _, err := os.Stat(outputPath); err != nil {
		t.Errorf("expected config file at %s, got error: %v", outputPath, err)
	}
}

func TestSmoke_VersionSubcommandExists(t *testing.T) {
	var found bool
	for _, sub := range rootCmd.Commands() {
		if sub.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'version' subcommand to be registered on rootCmd")
	}
}
