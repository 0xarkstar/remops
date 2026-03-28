package docker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/transport"
)

func TestComposePS_Success(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0, Stdout: `[{"Name":"web","State":"running"}]`}, nil
		},
	}
	out, err := NewDockerClient(mt).ComposePS(context.Background(), "web1", "/opt/mystack")
	if err != nil {
		t.Fatalf("ComposePS: unexpected error: %v", err)
	}
	if !strings.Contains(out, "running") {
		t.Errorf("ComposePS: expected output to contain 'running', got %q", out)
	}
}

func TestComposePS_NonZeroExit(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 1, Stderr: "no compose file found"}, nil
		},
	}
	_, err := NewDockerClient(mt).ComposePS(context.Background(), "web1", "/opt/missing")
	if err == nil {
		t.Fatal("ComposePS: expected error for non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "no compose file found") {
		t.Errorf("ComposePS: unexpected error message: %v", err)
	}
}

func TestComposeAction_Success(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0, Stdout: "Pulling images...\n"}, nil
		},
	}
	out, code, err := NewDockerClient(mt).ComposeAction(context.Background(), "web1", "/opt/mystack", "pull")
	if err != nil {
		t.Fatalf("ComposeAction: unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("ComposeAction: expected exit code 0, got %d", code)
	}
	if !strings.Contains(out, "Pulling") {
		t.Errorf("ComposeAction: expected output to contain 'Pulling', got %q", out)
	}
}

func TestComposeAction_NonZeroExit(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 1, Stderr: "image not found"}, nil
		},
	}
	out, code, err := NewDockerClient(mt).ComposeAction(context.Background(), "web1", "/opt/mystack", "up -d")
	if err != nil {
		t.Fatalf("ComposeAction: unexpected error: %v", err)
	}
	if code != 1 {
		t.Errorf("ComposeAction: expected exit code 1, got %d", code)
	}
	if !strings.Contains(out, "image not found") {
		t.Errorf("ComposeAction: expected stderr in output, got %q", out)
	}
}

func TestComposeAction_ExecError(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{}, errors.New("ssh disconnected")
		},
	}
	_, _, err := NewDockerClient(mt).ComposeAction(context.Background(), "web1", "/opt/mystack", "restart")
	if err == nil {
		t.Fatal("ComposeAction: expected error on exec failure, got nil")
	}
}

func TestComposeLogs_WithTailAndSince(t *testing.T) {
	var capturedCmd string
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, cmd string) (transport.ExecResult, error) {
			capturedCmd = cmd
			return transport.ExecResult{ExitCode: 0, Stdout: "2024-01-01 log line\n"}, nil
		},
	}
	out, err := NewDockerClient(mt).ComposeLogs(context.Background(), "web1", "/opt/mystack", 50, "1h", "web")
	if err != nil {
		t.Fatalf("ComposeLogs: unexpected error: %v", err)
	}
	if !strings.Contains(out, "log line") {
		t.Errorf("ComposeLogs: expected output to contain 'log line', got %q", out)
	}
	if !strings.Contains(capturedCmd, "--tail 50") {
		t.Errorf("ComposeLogs: expected --tail 50 in cmd, got %q", capturedCmd)
	}
	if !strings.Contains(capturedCmd, "--since 1h") {
		t.Errorf("ComposeLogs: expected --since 1h in cmd, got %q", capturedCmd)
	}
	if !strings.Contains(capturedCmd, " web") {
		t.Errorf("ComposeLogs: expected service name 'web' in cmd, got %q", capturedCmd)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/opt/mystack", "'/opt/mystack'"},
		{"/path/with spaces", "'/path/with spaces'"},
		{"/path/with'quote", "'/path/with'\\''quote'"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
