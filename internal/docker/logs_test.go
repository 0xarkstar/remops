package docker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/transport"
)

func TestBuildLogsCmd(t *testing.T) {
	tests := []struct {
		name      string
		container string
		opts      LogOptions
		wantParts []string
	}{
		{
			name:      "no opts",
			container: "myapp",
			opts:      LogOptions{},
			wantParts: []string{"docker logs myapp"},
		},
		{
			name:      "with tail",
			container: "myapp",
			opts:      LogOptions{Tail: "100"},
			wantParts: []string{"docker logs myapp", "--tail 100"},
		},
		{
			name:      "with since",
			container: "myapp",
			opts:      LogOptions{Since: "1h"},
			wantParts: []string{"docker logs myapp", "--since 1h"},
		},
		{
			name:      "with tail and since",
			container: "myapp",
			opts:      LogOptions{Tail: "50", Since: "2h"},
			wantParts: []string{"docker logs myapp", "--tail 50", "--since 2h"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := buildLogsCmd(tc.container, tc.opts)
			for _, part := range tc.wantParts {
				if !strings.Contains(cmd, part) {
					t.Errorf("buildLogsCmd: want %q in command, got %q", part, cmd)
				}
			}
		})
	}
}

func TestLogs(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "log line 1\nlog line 2\n", ExitCode: 0}, nil
		},
	}
	out, err := NewDockerClient(mt).Logs(context.Background(), "web1", "myapp", LogOptions{Tail: "10"})
	if err != nil {
		t.Fatalf("Logs: unexpected error: %v", err)
	}
	if !strings.Contains(out, "log line 1") {
		t.Errorf("Logs: expected log output, got %q", out)
	}
}

func TestLogsExecError(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{}, errors.New("connection lost")
		},
	}
	_, err := NewDockerClient(mt).Logs(context.Background(), "web1", "myapp", LogOptions{})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestLogsNonZeroExit(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 1, Stderr: "no such container"}, nil
		},
	}
	_, err := NewDockerClient(mt).Logs(context.Background(), "web1", "missing", LogOptions{})
	if err == nil {
		t.Error("expected error for non-zero exit, got nil")
	}
}

func TestStreamLogs(t *testing.T) {
	mt := &mockTransport{}
	rc, err := NewDockerClient(mt).StreamLogs(context.Background(), "web1", "myapp", LogOptions{Tail: "10"})
	if err != nil {
		t.Fatalf("StreamLogs: unexpected error: %v", err)
	}
	rc.Close()
}
