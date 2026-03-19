package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// LocalTransport runs commands on the local machine. Useful for testing.
type LocalTransport struct{}

// NewLocalTransport returns a Transport that executes commands locally.
func NewLocalTransport() Transport {
	return &LocalTransport{}
}

// Exec runs cmd locally and returns the combined result.
func (l *LocalTransport) Exec(ctx context.Context, _ string, cmd string) (ExecResult, error) {
	start := time.Now()

	args := strings.Fields(cmd)
	if len(args) == 0 {
		return ExecResult{}, fmt.Errorf("empty command")
	}

	c := exec.CommandContext(ctx, args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			err = nil // non-zero exit is not an error at this layer
		} else {
			return ExecResult{}, fmt.Errorf("exec: %w", err)
		}
	}

	return ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: time.Since(start),
		Host:     "localhost",
	}, nil
}

// Stream runs cmd locally and returns a reader over its combined output.
func (l *LocalTransport) Stream(ctx context.Context, _ string, cmd string) (io.ReadCloser, error) {
	args := strings.Fields(cmd)
	if len(args) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	c := exec.CommandContext(ctx, args[0], args[1:]...)
	pr, pw := io.Pipe()
	c.Stdout = pw
	c.Stderr = pw

	if err := c.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("stream start: %w", err)
	}

	go func() {
		pw.CloseWithError(c.Wait())
	}()

	return pr, nil
}

// Ping always succeeds for local execution.
func (l *LocalTransport) Ping(_ context.Context, host string) (PingResult, error) {
	return PingResult{Host: host, Latency: 0, Online: true}, nil
}

// Close is a no-op for local transport.
func (l *LocalTransport) Close() error { return nil }
