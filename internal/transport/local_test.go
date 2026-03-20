package transport

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestLocalTransportExec(t *testing.T) {
	lt := NewLocalTransport()
	result, err := lt.Exec(context.Background(), "localhost", "echo hello")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Errorf("stdout: want 'hello', got %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code: want 0, got %d", result.ExitCode)
	}
	if result.Host != "localhost" {
		t.Errorf("host: want localhost, got %s", result.Host)
	}
	if result.Duration <= 0 {
		t.Error("expected non-zero duration")
	}
}

func TestLocalTransportExecNonZeroExit(t *testing.T) {
	lt := NewLocalTransport()
	result, err := lt.Exec(context.Background(), "localhost", "false")
	if err != nil {
		t.Fatalf("Exec non-zero exit: unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("exit code: want non-zero, got 0")
	}
}

func TestLocalTransportExecStderr(t *testing.T) {
	lt := NewLocalTransport()
	// ls on a non-existent path writes to stderr and exits non-zero
	result, err := lt.Exec(context.Background(), "localhost", "ls /nonexistent_remops_test_path_xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
	if result.Stderr == "" {
		t.Error("expected non-empty stderr")
	}
}

func TestLocalTransportExecEmpty(t *testing.T) {
	lt := NewLocalTransport()
	_, err := lt.Exec(context.Background(), "localhost", "")
	if err == nil {
		t.Error("expected error for empty command, got nil")
	}
}

func TestLocalTransportExecCancelled(t *testing.T) {
	lt := NewLocalTransport()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := lt.Exec(ctx, "localhost", "sleep 10")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestLocalTransportStream(t *testing.T) {
	lt := NewLocalTransport()
	rc, err := lt.Stream(context.Background(), "localhost", "echo streaming")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(data), "streaming") {
		t.Errorf("stream output: expected 'streaming', got %q", string(data))
	}
}

func TestLocalTransportStreamEmpty(t *testing.T) {
	lt := NewLocalTransport()
	_, err := lt.Stream(context.Background(), "localhost", "")
	if err == nil {
		t.Error("expected error for empty command, got nil")
	}
}

func TestLocalTransportPing(t *testing.T) {
	lt := NewLocalTransport()
	result, err := lt.Ping(context.Background(), "myhost")
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !result.Online {
		t.Error("expected Online=true for local transport")
	}
	if result.Host != "myhost" {
		t.Errorf("host: want myhost, got %s", result.Host)
	}
}

func TestLocalTransportClose(t *testing.T) {
	lt := NewLocalTransport()
	if err := lt.Close(); err != nil {
		t.Errorf("Close: unexpected error: %v", err)
	}
}

func TestNewPool(t *testing.T) {
	p := NewPool()
	if p == nil {
		t.Fatal("expected non-nil pool")
	}
}

func TestPoolCloseEmpty(t *testing.T) {
	p := NewPool()
	if err := p.Close(); err != nil {
		t.Errorf("Close empty pool: %v", err)
	}
}
