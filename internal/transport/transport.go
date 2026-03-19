package transport

import (
	"context"
	"io"
	"time"
)

// Transport abstracts command execution on a host.
type Transport interface {
	Exec(ctx context.Context, host string, cmd string) (ExecResult, error)
	Stream(ctx context.Context, host string, cmd string) (io.ReadCloser, error)
	Ping(ctx context.Context, host string) (PingResult, error)
	Close() error
}

// ExecResult holds the outcome of a command execution.
type ExecResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration_ms"`
	Host     string        `json:"host"`
}

// PingResult holds the outcome of a connectivity check.
type PingResult struct {
	Host    string        `json:"host"`
	Latency time.Duration `json:"latency_ms"`
	Online  bool          `json:"online"`
}
