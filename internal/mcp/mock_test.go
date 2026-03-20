package mcp

import (
	"context"
	"io"
	"strings"

	"github.com/0xarkstar/remops/internal/transport"
)

type mockTransport struct {
	execFunc func(ctx context.Context, host, cmd string) (transport.ExecResult, error)
	pingFunc func(ctx context.Context, host string) (transport.PingResult, error)
}

func (m *mockTransport) Exec(ctx context.Context, host, cmd string) (transport.ExecResult, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, host, cmd)
	}
	return transport.ExecResult{}, nil
}

func (m *mockTransport) Stream(ctx context.Context, host, cmd string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockTransport) Ping(ctx context.Context, host string) (transport.PingResult, error) {
	if m.pingFunc != nil {
		return m.pingFunc(ctx, host)
	}
	return transport.PingResult{Host: host, Online: true}, nil
}

func (m *mockTransport) Close() error { return nil }
