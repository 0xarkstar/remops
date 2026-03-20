package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/transport"
)

func serviceConfig() *config.Config {
	return &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"web1": {Address: "1.1.1.1"},
		},
		Services: map[string]config.Service{
			"myapp": {Host: "web1", Container: "myapp_container"},
		},
		Profiles: map[string]config.Profile{},
	}
}

func TestMCPLogsShellInjection(t *testing.T) {
	s := NewServer(serviceConfig(), &mockTransport{})
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_logs",
		"arguments": map[string]any{"service": "myapp", "since": "; rm -rf /"},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error for shell injection in since, got nil")
	}
	if !strings.Contains(rpcErr.Message, "dangerous shell characters") {
		t.Errorf("expected 'dangerous shell characters' in error, got: %s", rpcErr.Message)
	}
}

func TestMCPLogsInvalidServiceName(t *testing.T) {
	s := NewServer(serviceConfig(), &mockTransport{})
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_logs",
		"arguments": map[string]any{"service": "foo;bar"},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error for invalid service name, got nil")
	}
	if !strings.Contains(rpcErr.Message, "invalid service name") {
		t.Errorf("expected 'invalid service name' in error, got: %s", rpcErr.Message)
	}
}

func TestMCPRestartInvalidServiceName(t *testing.T) {
	s := NewServer(serviceConfig(), &mockTransport{})
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_restart",
		"arguments": map[string]any{"service": "foo$(cmd)", "confirm": true},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error for invalid service name, got nil")
	}
	if !strings.Contains(rpcErr.Message, "invalid service name") {
		t.Errorf("expected 'invalid service name' in error, got: %s", rpcErr.Message)
	}
}

func TestMCPStatusInvalidHostName(t *testing.T) {
	s := NewServer(serviceConfig(), &mockTransport{})
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_status",
		"arguments": map[string]any{"host": "host;evil"},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error for invalid host name, got nil")
	}
	if !strings.Contains(rpcErr.Message, "invalid host name") {
		t.Errorf("expected 'invalid host name' in error, got: %s", rpcErr.Message)
	}
}

func TestMCPHostInfoInvalidHostName(t *testing.T) {
	s := NewServer(serviceConfig(), &mockTransport{})
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_host_info",
		"arguments": map[string]any{"host": "host|evil"},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error for invalid host name, got nil")
	}
	if !strings.Contains(rpcErr.Message, "invalid host name") {
		t.Errorf("expected 'invalid host name' in error, got: %s", rpcErr.Message)
	}
}

func TestMcpContentSanitizesOutput(t *testing.T) {
	result := mcpContent("normal text SYSTEM: injection attempt")
	content := result["content"].([]map[string]any)
	text := content[0]["text"].(string)
	if strings.Contains(text, "SYSTEM:") {
		t.Errorf("expected SYSTEM: to be redacted, got: %s", text)
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in output, got: %s", text)
	}
}

func TestMcpContentPreservesNormalOutput(t *testing.T) {
	result := mcpContent("normal container output")
	content := result["content"].([]map[string]any)
	text := content[0]["text"].(string)
	if text != "normal container output" {
		t.Errorf("expected output unchanged, got: %s", text)
	}
}

func TestMCPLogsValidSince(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "log line\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(serviceConfig(), mt)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_logs",
		"arguments": map[string]any{"service": "myapp", "since": "1h"},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected error for valid since value: %+v", rpcErr)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}
