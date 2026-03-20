// Package cmd - tests for run* handlers that can be exercised without SSH.
package cmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/spf13/cobra"
)

// newCmdWithContext returns a bare *cobra.Command with a background context.
func newCmdWithContext() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

// --- runSecurityScan early-exit paths ---

func TestRunSecurityScan_NoHosts(t *testing.T) {
	origCfg := cfg
	origHost := flagHost
	origTag := flagTag
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{},
	}
	flagHost = ""
	flagTag = ""
	t.Cleanup(func() {
		cfg = origCfg
		flagHost = origHost
		flagTag = origTag
	})

	err := runSecurityScan(newCmdWithContext(), nil)
	if err == nil {
		t.Fatal("expected error when no hosts")
	}
	if !strings.Contains(err.Error(), "no hosts") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunSecurityScan_UnknownHost(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
	}
	t.Cleanup(func() { cfg = origCfg })

	err := runSecurityScan(newCmdWithContext(), []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
	if !strings.Contains(err.Error(), "unknown host") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- runServiceWrite dry-run path ---

func TestRunServiceWrite_DryRun(t *testing.T) {
	origCfg := cfg
	origDryRun := flagDryRun
	origProfile := flagProfile
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Services: map[string]config.Service{
			"app": {Host: "prod", Container: "app_container"},
		},
		Profiles: map[string]config.Profile{
			"admin": {Level: "admin"},
		},
	}
	flagDryRun = true
	flagProfile = "admin"
	t.Cleanup(func() {
		cfg = origCfg
		flagDryRun = origDryRun
		flagProfile = origProfile
	})

	// Capture stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	handler := runServiceWrite("restart")
	cmd := &cobra.Command{}
	cmd.Flags().Bool("confirm", false, "")
	cmd.SetContext(context.Background())
	runErr := handler(cmd, []string{"app"})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if runErr != nil {
		t.Fatalf("runServiceWrite dry-run error: %v", runErr)
	}
	out := buf.String()
	if !strings.Contains(out, "dry-run") {
		t.Errorf("expected dry-run output, got: %s", out)
	}
}

func TestRunServiceWrite_UnknownService(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Services: map[string]config.Service{},
	}
	t.Cleanup(func() { cfg = origCfg })

	handler := runServiceWrite("restart")
	cmd := &cobra.Command{}
	cmd.Flags().Bool("confirm", false, "")
	cmd.SetContext(context.Background())
	err := handler(cmd, []string{"missing"})
	if err == nil {
		t.Fatal("expected error for unknown service")
	}
}

// --- runHostDisk early-exit paths ---

func TestRunHostDisk_UnknownHost(t *testing.T) {
	origCfg := cfg
	origProfile := flagProfile
	cfg = &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Profiles: map[string]config.Profile{"admin": {Level: "admin"}},
	}
	flagProfile = "admin"
	t.Cleanup(func() {
		cfg = origCfg
		flagProfile = origProfile
	})

	err := runHostDisk(newCmdWithContext(), []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
	if !strings.Contains(err.Error(), "unknown host") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- runHostPrune dry-run path ---

func TestRunHostPrune_UnknownHost(t *testing.T) {
	origCfg := cfg
	origProfile := flagProfile
	cfg = &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Profiles: map[string]config.Profile{"admin": {Level: "admin"}},
	}
	flagProfile = "admin"
	t.Cleanup(func() {
		cfg = origCfg
		flagProfile = origProfile
	})

	cmd := &cobra.Command{}
	cmd.Flags().Bool("confirm", false, "")
	cmd.Flags().Bool("volumes", false, "")
	cmd.SetContext(context.Background())
	err := runHostPrune(cmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
	if !strings.Contains(err.Error(), "unknown host") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- runHostExec early paths ---

func TestRunHostExec_UnknownHost(t *testing.T) {
	origCfg := cfg
	origProfile := flagProfile
	cfg = &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Profiles: map[string]config.Profile{"admin": {Level: "admin"}},
	}
	flagProfile = "admin"
	t.Cleanup(func() {
		cfg = origCfg
		flagProfile = origProfile
	})

	cmd := &cobra.Command{}
	cmd.Flags().Bool("confirm", false, "")
	cmd.SetContext(context.Background())
	err := runHostExec(cmd, []string{"nonexistent", "echo hello"})
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
	if !strings.Contains(err.Error(), "unknown host") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunHostExec_DryRun(t *testing.T) {
	origCfg := cfg
	origProfile := flagProfile
	origDryRun := flagDryRun
	cfg = &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Profiles: map[string]config.Profile{"admin": {Level: "admin"}},
	}
	flagProfile = "admin"
	flagDryRun = true
	t.Cleanup(func() {
		cfg = origCfg
		flagProfile = origProfile
		flagDryRun = origDryRun
	})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	cmd := &cobra.Command{}
	cmd.Flags().Bool("confirm", false, "")
	cmd.SetContext(context.Background())
	runErr := runHostExec(cmd, []string{"prod", "echo hello"})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if runErr != nil {
		t.Fatalf("runHostExec dry-run error: %v", runErr)
	}
	out := buf.String()
	if !strings.Contains(out, "dry-run") {
		t.Errorf("expected dry-run output, got: %s", out)
	}
}

func TestRunHostInfo_UnknownHost(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
	}
	t.Cleanup(func() { cfg = origCfg })

	err := runHostInfo(newCmdWithContext(), []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
	if !strings.Contains(err.Error(), "unknown host") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunServiceLogs_InvalidSince(t *testing.T) {
	origCfg := cfg
	origSince := flagServiceLogsSince
	origProfile := flagProfile
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Services: map[string]config.Service{
			"app": {Host: "prod", Container: "app_container"},
		},
		Profiles: map[string]config.Profile{"admin": {Level: "admin"}},
	}
	// Inject a shell injection character into --since to trigger the validation error
	flagServiceLogsSince = "1h; rm -rf /"
	flagProfile = "admin"
	t.Cleanup(func() {
		cfg = origCfg
		flagServiceLogsSince = origSince
		flagProfile = origProfile
	})

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runServiceLogs(cmd, []string{"app"})
	if err == nil {
		t.Fatal("expected error for shell injection in --since")
	}
	if !strings.Contains(err.Error(), "invalid --since") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunHostPrune_DryRunFlag(t *testing.T) {
	origCfg := cfg
	origProfile := flagProfile
	origDryRun := flagDryRun
	cfg = &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Profiles: map[string]config.Profile{"admin": {Level: "admin"}},
	}
	flagProfile = "admin"
	flagDryRun = false
	t.Cleanup(func() {
		cfg = origCfg
		flagProfile = origProfile
		flagDryRun = origDryRun
	})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	cmd := &cobra.Command{}
	cmd.Flags().Bool("confirm", false, "")
	cmd.Flags().Bool("volumes", false, "")
	cmd.SetContext(context.Background())
	// confirm=false, dry-run=false → prints dry-run message without executing
	runErr := runHostPrune(cmd, []string{"prod"})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if runErr != nil {
		t.Fatalf("runHostPrune no-confirm error: %v", runErr)
	}
	out := buf.String()
	if !strings.Contains(out, "dry-run") {
		t.Errorf("expected dry-run output, got: %s", out)
	}
}
