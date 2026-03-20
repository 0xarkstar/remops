package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"
)

type mockApprover struct {
	approved bool
	err      error
}

func (m *mockApprover) RequestApproval(_ context.Context, _ string) (bool, error) {
	return m.approved, m.err
}

func securityTestConfig() *config.Config {
	return &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"web1": {Address: "1.1.1.1"},
		},
		Services: map[string]config.Service{
			"myapp": {Host: "web1", Container: "myapp_c"},
		},
		Profiles: map[string]config.Profile{},
	}
}

func successTransport() *mockTransport {
	return &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0, Stdout: "myapp_c\n"}, nil
		},
	}
}

func callRestart(s *Server) *RPCError {
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_restart",
		"arguments": map[string]any{"service": "myapp", "confirm": true},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	return rpcErr
}

func TestServiceLifecycle_ViewerDenied(t *testing.T) {
	s := NewServer(securityTestConfig(), successTransport(), WithProfile(config.LevelViewer))
	rpcErr := callRestart(s)
	if rpcErr == nil {
		t.Fatal("expected permission denied error, got nil")
	}
	if !strings.Contains(rpcErr.Message, "permission denied") {
		t.Errorf("expected 'permission denied' in error, got: %s", rpcErr.Message)
	}
}

func TestServiceLifecycle_OperatorAllowed(t *testing.T) {
	s := NewServer(securityTestConfig(), successTransport(), WithProfile(config.LevelOperator))
	rpcErr := callRestart(s)
	if rpcErr != nil {
		t.Fatalf("expected success for operator, got: %+v", rpcErr)
	}
}

func TestServiceLifecycle_ApprovalDenied(t *testing.T) {
	s := NewServer(securityTestConfig(), successTransport(),
		WithProfile(config.LevelOperator),
		WithApprover(&mockApprover{approved: false}),
	)
	rpcErr := callRestart(s)
	if rpcErr == nil {
		t.Fatal("expected denial error, got nil")
	}
	if !strings.Contains(rpcErr.Message, "denied") {
		t.Errorf("expected 'denied' in error, got: %s", rpcErr.Message)
	}
}

func TestServiceLifecycle_ApprovalApproved(t *testing.T) {
	s := NewServer(securityTestConfig(), successTransport(),
		WithProfile(config.LevelOperator),
		WithApprover(&mockApprover{approved: true}),
	)
	rpcErr := callRestart(s)
	if rpcErr != nil {
		t.Fatalf("expected success when approved, got: %+v", rpcErr)
	}
}

func TestServiceLifecycle_RateLimited(t *testing.T) {
	rl, err := security.NewRateLimiter(2)
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}
	// Pre-populate two records so the next Check fails.
	_ = rl.Record("web1", "docker restart myapp_c")
	_ = rl.Record("web1", "docker restart myapp_c")

	s := NewServer(securityTestConfig(), successTransport(),
		WithProfile(config.LevelOperator),
		WithRateLimiter(rl),
	)
	rpcErr := callRestart(s)
	if rpcErr == nil {
		t.Fatal("expected rate limit error, got nil")
	}
	if !strings.Contains(rpcErr.Message, "rate limit") {
		t.Errorf("expected 'rate limit' in error, got: %s", rpcErr.Message)
	}
}
