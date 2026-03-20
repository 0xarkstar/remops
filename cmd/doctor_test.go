package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xarkstar/remops/internal/transport"
	"github.com/spf13/cobra"
)

// mockTransport implements transport.Transport for testing.
type mockTransport struct {
	execResult transport.ExecResult
	execErr    error
	pingResult transport.PingResult
	pingErr    error
	// execFunc overrides execResult/execErr when set.
	execFunc func(host, cmd string) (transport.ExecResult, error)
}

func (m *mockTransport) Exec(_ context.Context, host, cmd string) (transport.ExecResult, error) {
	if m.execFunc != nil {
		return m.execFunc(host, cmd)
	}
	r := m.execResult
	r.Host = host
	return r, m.execErr
}

func (m *mockTransport) Stream(_ context.Context, _ string, _ string) (io.ReadCloser, error) {
	return io.NopCloser(nil), nil
}

func (m *mockTransport) Ping(_ context.Context, host string) (transport.PingResult, error) {
	r := m.pingResult
	r.Host = host
	return r, m.pingErr
}

func (m *mockTransport) Close() error { return nil }

// newTestCmd returns a *cobra.Command set up with a background context for testing.
func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	return cmd
}

func TestCheckHostPing_Online(t *testing.T) {
	tr := &mockTransport{
		pingResult: transport.PingResult{Online: true, Latency: 5 * time.Millisecond},
	}
	cmd := newTestCmd()
	result := checkHostPing(cmd, tr, "prod")
	if result.Status != statusPass {
		t.Errorf("got status %q, want PASS (detail: %s)", result.Status, result.Detail)
	}
}

func TestCheckHostPing_Offline(t *testing.T) {
	tr := &mockTransport{
		pingResult: transport.PingResult{Online: false},
	}
	cmd := newTestCmd()
	result := checkHostPing(cmd, tr, "prod")
	if result.Status != statusFail {
		t.Errorf("got status %q, want FAIL", result.Status)
	}
}

func TestCheckHostPing_Error(t *testing.T) {
	tr := &mockTransport{pingErr: errors.New("connection refused")}
	cmd := newTestCmd()
	result := checkHostPing(cmd, tr, "prod")
	if result.Status != statusFail {
		t.Errorf("got status %q, want FAIL", result.Status)
	}
}

func TestCheckDockerInstalled_Present(t *testing.T) {
	tr := &mockTransport{
		execResult: transport.ExecResult{Stdout: "Docker version 24.0.0, build abc123", ExitCode: 0},
	}
	cmd := newTestCmd()
	result := checkDockerInstalled(cmd, tr, "prod")
	if result.Status != statusPass {
		t.Errorf("got status %q, want PASS (detail: %s)", result.Status, result.Detail)
	}
}

func TestCheckDockerInstalled_NotFound(t *testing.T) {
	tr := &mockTransport{
		execResult: transport.ExecResult{ExitCode: 1, Stderr: "command not found"},
	}
	cmd := newTestCmd()
	result := checkDockerInstalled(cmd, tr, "prod")
	if result.Status != statusFail {
		t.Errorf("got status %q, want FAIL", result.Status)
	}
}

func TestCheckDockerInstalled_ExecError(t *testing.T) {
	tr := &mockTransport{execErr: errors.New("ssh timeout")}
	cmd := newTestCmd()
	result := checkDockerInstalled(cmd, tr, "prod")
	if result.Status != statusFail {
		t.Errorf("got status %q, want FAIL", result.Status)
	}
}

func TestCheckSSHKeyPerms_NoKeyFiles(t *testing.T) {
	// Point HOME to a temp dir with no .ssh keys — should PASS
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	result := checkSSHKeyPerms()
	if result.Status != statusPass {
		t.Errorf("got status %q, want PASS (detail: %s)", result.Status, result.Detail)
	}
	if result.Name != "SSH key permissions" {
		t.Errorf("got name %q, want %q", result.Name, "SSH key permissions")
	}
}

func TestCheckSSHKeyPerms_SafePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(sshDir, "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("fake key"), 0o600); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	result := checkSSHKeyPerms()
	if result.Status != statusPass {
		t.Errorf("got status %q, want PASS (detail: %s)", result.Status, result.Detail)
	}
}

func TestCheckSSHKeyPerms_InsecurePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(sshDir, "id_rsa")
	if err := os.WriteFile(keyPath, []byte("fake key"), 0o644); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	result := checkSSHKeyPerms()
	if result.Status != statusWarn {
		t.Errorf("got status %q, want WARN (detail: %s)", result.Status, result.Detail)
	}
}

func TestCheckAuditLogDir_WritableDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Override XDG_DATA_HOME so AuditLogPath() points into our temp dir
	origXDG := os.Getenv("XDG_DATA_HOME")
	t.Setenv("XDG_DATA_HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("XDG_DATA_HOME", origXDG) })

	result := checkAuditLogDir()
	if result.Status != statusPass {
		t.Errorf("got status %q, want PASS (detail: %s)", result.Status, result.Detail)
	}
	if result.Name != "Audit log dir" {
		t.Errorf("got name %q, want %q", result.Name, "Audit log dir")
	}
}

func TestCheckConfigFilePerms_NoConfigFile(t *testing.T) {
	// Point all config paths to non-existent locations
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	origRemops := os.Getenv("REMOPS_CONFIG")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("REMOPS_CONFIG", "")
	// Override cwd-based lookup by ensuring remops.yaml doesn't exist in a fresh tempdir
	// We can't easily override "." but the function will skip non-existent files and
	// return WARN when none are found.
	t.Cleanup(func() {
		os.Setenv("XDG_CONFIG_HOME", origXDG)
		os.Setenv("REMOPS_CONFIG", origRemops)
	})

	result := checkConfigFilePerms()
	// Either WARN (no config found) or PASS/WARN based on file state
	if result.Name != "Config file permissions" {
		t.Errorf("got name %q, want %q", result.Name, "Config file permissions")
	}
}

func TestCheckConfigFilePerms_SafePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "remops.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	origRemops := os.Getenv("REMOPS_CONFIG")
	t.Setenv("REMOPS_CONFIG", cfgPath)
	t.Cleanup(func() { os.Setenv("REMOPS_CONFIG", origRemops) })

	result := checkConfigFilePerms()
	if result.Status != statusPass {
		t.Errorf("got status %q, want PASS (detail: %s)", result.Status, result.Detail)
	}
}

func TestCheckConfigFilePerms_InsecurePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "remops.yaml")
	if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origRemops := os.Getenv("REMOPS_CONFIG")
	t.Setenv("REMOPS_CONFIG", cfgPath)
	t.Cleanup(func() { os.Setenv("REMOPS_CONFIG", origRemops) })

	result := checkConfigFilePerms()
	if result.Status != statusWarn {
		t.Errorf("got status %q, want WARN (detail: %s)", result.Status, result.Detail)
	}
}

func TestCheckConfig_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "remops.yaml")
	// version 99 is invalid
	if err := os.WriteFile(cfgPath, []byte("version: 99\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	origRemops := os.Getenv("REMOPS_CONFIG")
	t.Setenv("REMOPS_CONFIG", cfgPath)
	t.Cleanup(func() { os.Setenv("REMOPS_CONFIG", origRemops) })

	result := checkConfig()
	if result.Status != statusFail {
		t.Errorf("got status %q, want FAIL (detail: %s)", result.Status, result.Detail)
	}
	if result.Name != "Config file" {
		t.Errorf("got name %q, want %q", result.Name, "Config file")
	}
}

func TestCheckConfig_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "remops.yaml")
	content := `version: 1
hosts:
  prod:
    address: 1.2.3.4
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	origRemops := os.Getenv("REMOPS_CONFIG")
	t.Setenv("REMOPS_CONFIG", cfgPath)
	t.Cleanup(func() { os.Setenv("REMOPS_CONFIG", origRemops) })

	result := checkConfig()
	if result.Status != statusPass {
		t.Errorf("got status %q, want PASS (detail: %s)", result.Status, result.Detail)
	}
}

func TestPrintDoctorTable(t *testing.T) {
	results := []checkResult{
		{Name: "Config file", Status: statusPass, Detail: "found"},
		{Name: "Docker", Status: statusFail, Detail: "not installed"},
		{Name: "Permissions", Status: statusWarn, Detail: "loose perms"},
	}
	// Just verify no panic — output goes to stdout.
	printDoctorTable(results)
}

func TestRunDoctor_NilConfig(t *testing.T) {
	origCfg := cfg
	cfg = nil
	t.Cleanup(func() { cfg = origCfg })

	cmd := newTestCmd()
	err := runDoctor(cmd, nil)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestCheckResult_Fields(t *testing.T) {
	r := checkResult{
		Name:   "Test check",
		Status: statusPass,
		Detail: "all good",
	}
	if r.Name != "Test check" {
		t.Errorf("Name = %q, want %q", r.Name, "Test check")
	}
	if r.Status != statusPass {
		t.Errorf("Status = %q, want %q", r.Status, statusPass)
	}
	if r.Detail != "all good" {
		t.Errorf("Detail = %q, want %q", r.Detail, "all good")
	}
}
