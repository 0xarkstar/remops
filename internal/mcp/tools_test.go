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

func TestMcpContent(t *testing.T) {
	result := mcpContent("hello world")
	content, ok := result["content"]
	if !ok {
		t.Fatal("mcpContent: missing 'content' key")
	}
	items, ok := content.([]map[string]any)
	if !ok {
		t.Fatalf("content: want []map[string]any, got %T", content)
	}
	if len(items) != 1 {
		t.Fatalf("content: want 1 item, got %d", len(items))
	}
	if items[0]["text"] != "hello world" {
		t.Errorf("text: want 'hello world', got %v", items[0]["text"])
	}
	if items[0]["type"] != "text" {
		t.Errorf("type: want 'text', got %v", items[0]["type"])
	}
}

func TestResolveHostsByName(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	hosts := resolveHosts(s, "web1", "")
	if len(hosts) != 1 || hosts[0] != "web1" {
		t.Errorf("resolveHosts(web1): want [web1], got %v", hosts)
	}
}

func TestResolveHostsUnknownName(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	hosts := resolveHosts(s, "nope", "")
	if len(hosts) != 0 {
		t.Errorf("resolveHosts(nope): want [], got %v", hosts)
	}
}

func TestResolveHostsByTag(t *testing.T) {
	cfg := &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"web1": {Address: "1.1.1.1", Tags: []string{"prod"}},
			"db1":  {Address: "1.1.1.2", Tags: []string{"staging"}},
		},
		Services: map[string]config.Service{},
		Profiles: map[string]config.Profile{},
	}
	s := NewServer(cfg, nil)
	hosts := resolveHosts(s, "", "prod")
	if len(hosts) != 1 || hosts[0] != "web1" {
		t.Errorf("resolveHosts(tag=prod): want [web1], got %v", hosts)
	}
}

func TestResolveHostsAll(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	hosts := resolveHosts(s, "", "")
	if len(hosts) != 1 {
		t.Errorf("resolveHosts(all): want 1 host, got %v", hosts)
	}
}

func TestResolveHostsNilConfig(t *testing.T) {
	s := &Server{config: nil}
	hosts := resolveHosts(s, "", "")
	if hosts != nil {
		t.Errorf("resolveHosts(nil config): want nil, got %v", hosts)
	}
}

func TestDispatchToolsCallStatus(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, host, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{
				Stdout:   `{"Names":"myapp","State":"running"}` + "\n",
				ExitCode: 0,
			}, nil
		},
	}
	s := NewServer(minimalConfig(), mt)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_status",
		"arguments": map[string]any{"host": "web1"},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestDispatchToolsCallServiceLogs(t *testing.T) {
	cfg := &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"web1": {Address: "1.1.1.1"},
		},
		Services: map[string]config.Service{
			"myapp": {Host: "web1", Container: "myapp_container"},
		},
		Profiles: map[string]config.Profile{},
	}
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "log output\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(cfg, mt)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_logs",
		"arguments": map[string]any{"service": "myapp", "tail": 10},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	content := m["content"].([]map[string]any)
	if !strings.Contains(content[0]["text"].(string), "log output") {
		t.Errorf("expected log output in result, got %v", content[0]["text"])
	}
}

func TestDispatchToolsCallServiceRestartNoConfirm(t *testing.T) {
	cfg := &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"web1": {Address: "1.1.1.1"}},
		Services: map[string]config.Service{"myapp": {Host: "web1", Container: "myapp_container"}},
		Profiles: map[string]config.Profile{},
	}
	s := NewServer(cfg, nil)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_restart",
		"arguments": map[string]any{"service": "myapp", "confirm": false},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error when confirm=false, got nil")
	}
}

func TestDispatchToolsCallServiceRestartUnknownService(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_restart",
		"arguments": map[string]any{"service": "unknown", "confirm": true},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error for unknown service, got nil")
	}
}

func TestDispatchToolsCallServiceLogsUnknownService(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_logs",
		"arguments": map[string]any{"service": "no_such_service"},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error for unknown service, got nil")
	}
}

func TestDispatchToolsCallDoctor(t *testing.T) {
	mt := &mockTransport{
		pingFunc: func(_ context.Context, host string) (transport.PingResult, error) {
			return transport.PingResult{Host: host, Online: true, Latency: 1000000}, nil
		},
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "24.0.0\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(minimalConfig(), mt)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_doctor",
		"arguments": map[string]any{},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestDispatchToolsCallHostInfo(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "Linux web1 5.15.0\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(minimalConfig(), mt)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_host_info",
		"arguments": map[string]any{"host": "web1"},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestDispatchToolsCallServiceStop(t *testing.T) {
	cfg := &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"web1": {Address: "1.1.1.1"}},
		Services: map[string]config.Service{"myapp": {Host: "web1", Container: "myapp_c"}},
		Profiles: map[string]config.Profile{},
	}
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0, Stdout: "myapp_c\n"}, nil
		},
	}
	s := NewServer(cfg, mt)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_stop",
		"arguments": map[string]any{"service": "myapp", "confirm": true},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

func TestDispatchToolsCallServiceStart(t *testing.T) {
	cfg := &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"web1": {Address: "1.1.1.1"}},
		Services: map[string]config.Service{"myapp": {Host: "web1", Container: "myapp_c"}},
		Profiles: map[string]config.Profile{},
	}
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0, Stdout: "myapp_c\n"}, nil
		},
	}
	s := NewServer(cfg, mt)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_start",
		"arguments": map[string]any{"service": "myapp", "confirm": true},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if result == nil {
		t.Fatal("expected result")
	}
}

func TestDispatchToolsCallStatusNoHosts(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_status",
		"arguments": map[string]any{"tag": "nonexistent-tag"},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	// Should return "no hosts matched"
	m := result.(map[string]any)
	content := m["content"].([]map[string]any)
	if !strings.Contains(content[0]["text"].(string), "no hosts matched") {
		t.Errorf("expected 'no hosts matched', got %v", content[0]["text"])
	}
}

func dbConfig() *config.Config {
	return &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{"web1": {Address: "1.1.1.1"}},
		Services: map[string]config.Service{
			"myapp": {Host: "web1", Container: "myapp_c", DB: &config.DBConfig{
				Engine: "postgresql", User: "admin", Database: "mydb",
			}},
			"nodbapp": {Host: "web1", Container: "nodbapp_c"},
		},
		Profiles: map[string]config.Profile{},
	}
}

func TestDBQuery_Success(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: " count \n-------\n     1\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(dbConfig(), mt)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_db_query",
		"arguments": map[string]any{"service": "myapp", "query": "SELECT 1"},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	content := m["content"].([]map[string]any)
	if !strings.Contains(content[0]["text"].(string), "count") {
		t.Errorf("expected query output in result, got %v", content[0]["text"])
	}
}

func TestDBQuery_NoDBConfig(t *testing.T) {
	s := NewServer(dbConfig(), nil)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_db_query",
		"arguments": map[string]any{"service": "nodbapp", "query": "SELECT 1"},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error for service with no db config, got nil")
	}
	if !strings.Contains(rpcErr.Message, "no db config") {
		t.Errorf("expected 'no db config' in error, got: %s", rpcErr.Message)
	}
}

func TestDBQuery_WriteQueryDenied(t *testing.T) {
	s := NewServer(dbConfig(), nil, WithProfile(config.LevelViewer))
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_db_query",
		"arguments": map[string]any{"service": "myapp", "query": "INSERT INTO t VALUES (1)"},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected permission error for write query as viewer, got nil")
	}
	if !strings.Contains(rpcErr.Message, "permission") {
		t.Errorf("expected 'permission' in error, got: %s", rpcErr.Message)
	}
}

func TestDBQuery_WriteQueryAllowedForOperator(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "INSERT 0 1\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(dbConfig(), mt, WithProfile(config.LevelOperator))
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_db_query",
		"arguments": map[string]any{"service": "myapp", "query": "INSERT INTO t VALUES (1)"},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("expected success for operator write query, got: %+v", rpcErr)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestDBQuery_UnsupportedEngine(t *testing.T) {
	// Build a config with an unsupported engine by bypassing validation via direct struct.
	cfg := &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"web1": {Address: "1.1.1.1"}},
		Services: map[string]config.Service{
			"myapp": {Host: "web1", Container: "myapp_c", DB: &config.DBConfig{
				Engine: "sqlite", User: "admin", Database: "mydb",
			}},
		},
		Profiles: map[string]config.Profile{},
	}
	s := NewServer(cfg, nil)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_db_query",
		"arguments": map[string]any{"service": "myapp", "query": "SELECT 1"},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error for unsupported engine, got nil")
	}
	if !strings.Contains(rpcErr.Message, "unsupported") {
		t.Errorf("expected 'unsupported' in error, got: %s", rpcErr.Message)
	}
}

func TestHostExec_Success(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, cmd string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "hello\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(minimalConfig(), mt, WithProfile(config.LevelAdmin))
	params, _ := json.Marshal(map[string]any{
		"name": "remops_host_exec",
		"arguments": map[string]any{
			"host":    "web1",
			"command": "echo hello",
			"confirm": true,
		},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	content := m["content"].([]map[string]any)
	if !strings.Contains(content[0]["text"].(string), "hello") {
		t.Errorf("expected 'hello' in result, got %v", content[0]["text"])
	}
}

func TestHostExec_ViewerDenied(t *testing.T) {
	s := NewServer(minimalConfig(), nil, WithProfile(config.LevelViewer))
	params, _ := json.Marshal(map[string]any{
		"name": "remops_host_exec",
		"arguments": map[string]any{
			"host":    "web1",
			"command": "echo hello",
			"confirm": true,
		},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected permission denied error, got nil")
	}
	if !strings.Contains(rpcErr.Message, "permission denied") {
		t.Errorf("expected 'permission denied' in error, got: %s", rpcErr.Message)
	}
}

func TestHostExec_NoConfirm(t *testing.T) {
	s := NewServer(minimalConfig(), nil, WithProfile(config.LevelAdmin))
	params, _ := json.Marshal(map[string]any{
		"name": "remops_host_exec",
		"arguments": map[string]any{
			"host":    "web1",
			"command": "echo hello",
			"confirm": false,
		},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected error when confirm=false, got nil")
	}
	if !strings.Contains(rpcErr.Message, "confirm") {
		t.Errorf("expected 'confirm' in error, got: %s", rpcErr.Message)
	}
}

func TestHostDisk_Success(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "Filesystem  Size  Used Avail Use%\n/dev/sda1   50G   10G   40G  20%\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(minimalConfig(), mt)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_host_disk",
		"arguments": map[string]any{"host": "web1"},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	content := m["content"].([]map[string]any)
	if !strings.Contains(content[0]["text"].(string), "sda1") {
		t.Errorf("expected disk output in result, got %v", content[0]["text"])
	}
}

func TestHostDisk_InvalidHost(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_host_disk",
		"arguments": map[string]any{"host": "bad;host$(evil)"},
	})
	_, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr == nil {
		t.Fatal("expected validation error for invalid host, got nil")
	}
}

func TestWithVersion(t *testing.T) {
	s := NewServer(minimalConfig(), nil, WithVersion("1.2.3"))
	if s.version != "1.2.3" {
		t.Errorf("WithVersion: want '1.2.3', got %q", s.version)
	}
}

func TestWithAuditLogger(t *testing.T) {
	al, err := security.NewAuditLogger()
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	s := NewServer(minimalConfig(), nil, WithAuditLogger(al))
	if s.auditLogger != al {
		t.Errorf("WithAuditLogger: auditLogger not set correctly")
	}
}

func TestServiceLifecycleWithConfirm(t *testing.T) {
	cfg := &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"web1": {Address: "1.1.1.1"}},
		Services: map[string]config.Service{"myapp": {Host: "web1", Container: "myapp_c"}},
		Profiles: map[string]config.Profile{},
	}
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0, Stdout: "myapp_c\n"}, nil
		},
	}
	s := NewServer(cfg, mt)
	params, _ := json.Marshal(map[string]any{
		"name":      "remops_service_restart",
		"arguments": map[string]any{"service": "myapp", "confirm": true},
	})
	result, rpcErr := s.dispatch(context.Background(), &JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	})
	if rpcErr != nil {
		t.Fatalf("unexpected rpc error: %+v", rpcErr)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}
