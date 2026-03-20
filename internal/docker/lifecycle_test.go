package docker

import (
	"context"
	"errors"
	"testing"

	"github.com/0xarkstar/remops/internal/transport"
)

func TestRestart(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0}, nil
		},
	}
	if err := NewDockerClient(mt).Restart(context.Background(), "web1", "myapp"); err != nil {
		t.Errorf("Restart: unexpected error: %v", err)
	}
}

func TestStop(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0}, nil
		},
	}
	if err := NewDockerClient(mt).Stop(context.Background(), "web1", "myapp"); err != nil {
		t.Errorf("Stop: unexpected error: %v", err)
	}
}

func TestStart(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0}, nil
		},
	}
	if err := NewDockerClient(mt).Start(context.Background(), "web1", "myapp"); err != nil {
		t.Errorf("Start: unexpected error: %v", err)
	}
}

func TestLifecycleExecError(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{}, errors.New("ssh disconnected")
		},
	}
	if err := NewDockerClient(mt).Restart(context.Background(), "web1", "myapp"); err == nil {
		t.Error("expected error on exec failure, got nil")
	}
}

func TestLifecycleNonZeroExit(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 1, Stderr: "no such container"}, nil
		},
	}
	if err := NewDockerClient(mt).Stop(context.Background(), "web1", "missing"); err == nil {
		t.Error("expected error for non-zero exit, got nil")
	}
}
