package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
)

func minimalConfig() *config.Config {
	return &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"web1": {Address: "192.168.1.1"},
		},
		Services: map[string]config.Service{},
		Profiles: map[string]config.Profile{},
	}
}

func TestDispatchInitialize(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "initialize",
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if _, ok := m["serverInfo"]; !ok {
		t.Error("expected 'serverInfo' in result")
	}
	if _, ok := m["protocolVersion"]; !ok {
		t.Error("expected 'protocolVersion' in result")
	}
	if _, ok := m["capabilities"]; !ok {
		t.Error("expected 'capabilities' in result")
	}
}

func TestDispatchToolsList(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/list",
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	tools, ok := m["tools"]
	if !ok {
		t.Fatal("expected 'tools' key in result")
	}
	defs, ok := tools.([]ToolDef)
	if !ok {
		t.Fatalf("expected []ToolDef, got %T", tools)
	}
	if len(defs) == 0 {
		t.Error("expected at least one tool definition")
	}
	// Verify expected tool names are present
	toolNames := make(map[string]bool)
	for _, d := range defs {
		toolNames[d.Name] = true
	}
	for _, expected := range []string{"remops_status", "remops_service_logs", "remops_doctor"} {
		if !toolNames[expected] {
			t.Errorf("expected tool %q to be registered", expected)
		}
	}
}

func TestDispatchUnknownMethod(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "unknown/method",
	})
	if rpcErr == nil {
		t.Fatal("expected rpc error for unknown method, got nil")
	}
	if rpcErr.Code != -32601 {
		t.Errorf("error code: want -32601, got %d", rpcErr.Code)
	}
}

func TestDispatchToolsCallUnknownTool(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	params, _ := json.Marshal(map[string]any{
		"name":      "no_such_tool",
		"arguments": json.RawMessage(`{}`),
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected rpc error for unknown tool, got nil")
	}
	if rpcErr.Code != -32601 {
		t.Errorf("error code: want -32601, got %d", rpcErr.Code)
	}
}

func TestDispatchToolsCallInvalidParams(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: json.RawMessage(`{invalid json`),
	})
	if rpcErr == nil {
		t.Fatal("expected rpc error for invalid params, got nil")
	}
	if rpcErr.Code != -32602 {
		t.Errorf("error code: want -32602, got %d", rpcErr.Code)
	}
}

func TestDispatchNotificationsInitialized(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "notifications/initialized",
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if result != nil {
		t.Errorf("expected nil result for notification ack, got %v", result)
	}
}
