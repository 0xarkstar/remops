// Package smoketest holds cross-interface integration tests that verify the MCP
// server and HTTP API server produce consistent results when driven by the same
// config and mock transport.
package smoketest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/api"
	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/mcp"
	"github.com/0xarkstar/remops/internal/transport"
)

// sharedConfig returns a config used by both servers in every cross-interface
// test. Two hosts are included so the "all hosts" status path is exercised.
func sharedConfig() *config.Config {
	return &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod":    {Address: "10.0.0.1"},
			"staging": {Address: "10.0.0.2"},
		},
		Services: map[string]config.Service{
			"webapp": {Host: "prod", Container: "webapp_c"},
		},
		Profiles: map[string]config.Profile{},
		API:      &config.APIConfig{APIKey: "ci-test-key"},
	}
}

// fixedDockerPS is the raw `docker ps --format '{{json .}}'` output the mock
// transport returns for every host.
const fixedDockerPS = `{"ID":"abc123","Image":"nginx:latest","Names":"webapp_c","Status":"Up 5 minutes"}
`

// sharedMockTransport returns a transport whose Exec always responds with
// fixedDockerPS, simulating a live host.
func sharedMockTransport() transport.Transport {
	return &mockTransport{
		execFunc: func(_ context.Context, host, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: fixedDockerPS, Host: host}, nil
		},
	}
}

// mockTransport is a local test double; duplicated here so the smoketest
// package has no dependency on internal package test helpers.
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

func (m *mockTransport) Stream(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockTransport) Ping(ctx context.Context, host string) (transport.PingResult, error) {
	if m.pingFunc != nil {
		return m.pingFunc(ctx, host)
	}
	return transport.PingResult{Host: host, Online: true}, nil
}

func (m *mockTransport) Close() error { return nil }

// --- helpers -----------------------------------------------------------------

// callMCPStatus dispatches the remops_status tool call directly on an MCP
// Server and returns the text content from the MCP envelope.
func callMCPStatus(t *testing.T, srv *mcp.Server, hostFilter string) string {
	t.Helper()

	args := map[string]string{}
	if hostFilter != "" {
		args["host"] = hostFilter
	}
	rawArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal MCP args: %v", err)
	}

	params, err := json.Marshal(map[string]any{
		"name":      "remops_status",
		"arguments": json.RawMessage(rawArgs),
	})
	if err != nil {
		t.Fatalf("marshal MCP params: %v", err)
	}

	req := &mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(params),
	}

	result, rpcErr := srv.Dispatch(context.Background(), req)
	if rpcErr != nil {
		t.Fatalf("MCP dispatch error: code=%d msg=%s", rpcErr.Code, rpcErr.Message)
	}

	return extractMCPText(t, result)
}

// extractMCPText pulls the first text content item from an MCP tool result.
func extractMCPText(t *testing.T, result any) string {
	t.Helper()
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("MCP result: want map[string]any, got %T", result)
	}
	items, ok := m["content"].([]map[string]any)
	if !ok {
		t.Fatalf("MCP result content: want []map[string]any, got %T", m["content"])
	}
	if len(items) == 0 {
		t.Fatal("MCP result content: empty")
	}
	text, _ := items[0]["text"].(string)
	return text
}

// callHTTPStatus fires GET /api/v1/status against the API server's Handler and
// returns the decoded response body.
func callHTTPStatus(t *testing.T, srv *api.Server, hostFilter string) map[string]any {
	t.Helper()

	target := "/api/v1/status"
	if hostFilter != "" {
		target += "?host=" + hostFilter
	}

	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", "Bearer ci-test-key")

	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("HTTP status: want 200, got %d — body: %s", resp.StatusCode, body)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode HTTP response: %v", err)
	}
	return out
}

// --- tests -------------------------------------------------------------------

// TestCrossInterface_StatusConsistency verifies that both interfaces report the
// same host names when querying all hosts via the status operation.
func TestCrossInterface_StatusConsistency(t *testing.T) {
	cfg := sharedConfig()
	tr := sharedMockTransport()

	mcpSrv := mcp.NewServer(cfg, tr)
	httpSrv := api.NewServer(cfg, tr)

	mcpText := callMCPStatus(t, mcpSrv, "")
	httpBody := callHTTPStatus(t, httpSrv, "")

	// MCP produces a text block with "# <hostname>" markers.
	if !strings.Contains(mcpText, "# prod") {
		t.Errorf("MCP status: expected host 'prod' in output, got:\n%s", mcpText)
	}
	if !strings.Contains(mcpText, "# staging") {
		t.Errorf("MCP status: expected host 'staging' in output, got:\n%s", mcpText)
	}

	// HTTP produces a JSON array of host objects.
	hosts, ok := httpBody["hosts"].([]any)
	if !ok {
		t.Fatalf("HTTP response missing 'hosts' array, got keys: %v", mapKeys(httpBody))
	}
	if len(hosts) != 2 {
		t.Errorf("HTTP status: expected 2 hosts, got %d", len(hosts))
	}

	httpHostNames := make(map[string]bool)
	for _, h := range hosts {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := hm["host"].(string); ok {
			httpHostNames[name] = true
		}
	}

	for _, expected := range []string{"prod", "staging"} {
		if !httpHostNames[expected] {
			t.Errorf("HTTP status: expected host %q in response, got hosts: %v", expected, httpHostNames)
		}
	}
}

// TestCrossInterface_SingleHostFilter verifies that filtering by a specific
// host name produces consistent scoping across MCP and HTTP.
func TestCrossInterface_SingleHostFilter(t *testing.T) {
	cfg := sharedConfig()
	tr := sharedMockTransport()

	mcpSrv := mcp.NewServer(cfg, tr)
	httpSrv := api.NewServer(cfg, tr)

	mcpText := callMCPStatus(t, mcpSrv, "prod")
	httpBody := callHTTPStatus(t, httpSrv, "prod")

	// MCP should contain prod but NOT staging.
	if !strings.Contains(mcpText, "# prod") {
		t.Errorf("MCP single-host: expected 'prod' in output, got:\n%s", mcpText)
	}
	if strings.Contains(mcpText, "# staging") {
		t.Errorf("MCP single-host: unexpected 'staging' when filtered to 'prod':\n%s", mcpText)
	}

	// HTTP should return exactly one host entry.
	hosts, ok := httpBody["hosts"].([]any)
	if !ok {
		t.Fatalf("HTTP response missing 'hosts' array")
	}
	if len(hosts) != 1 {
		t.Errorf("HTTP single-host: expected 1 host, got %d", len(hosts))
	}
	hm, ok := hosts[0].(map[string]any)
	if !ok {
		t.Fatal("HTTP host entry is not a map")
	}
	if hm["host"] != "prod" {
		t.Errorf("HTTP single-host: expected host 'prod', got %v", hm["host"])
	}
}

// TestCrossInterface_DockerOutputPresent verifies that both interfaces surface
// the fixed docker ps output returned by the shared mock transport.
func TestCrossInterface_DockerOutputPresent(t *testing.T) {
	cfg := sharedConfig()
	tr := sharedMockTransport()

	mcpSrv := mcp.NewServer(cfg, tr)
	httpSrv := api.NewServer(cfg, tr)

	mcpText := callMCPStatus(t, mcpSrv, "prod")
	httpBody := callHTTPStatus(t, httpSrv, "prod")

	// The container ID from fixedDockerPS must appear in both responses.
	const containerID = "abc123"

	if !strings.Contains(mcpText, containerID) {
		t.Errorf("MCP output: expected container ID %q, got:\n%s", containerID, mcpText)
	}

	hosts, _ := httpBody["hosts"].([]any)
	if len(hosts) == 0 {
		t.Fatal("HTTP response has no hosts")
	}
	hm, _ := hosts[0].(map[string]any)
	output, _ := hm["output"].(string)
	if !strings.Contains(output, containerID) {
		t.Errorf("HTTP output: expected container ID %q, got:\n%s", containerID, output)
	}
}

// TestCrossInterface_UnknownHostReturnsEmpty verifies that both interfaces
// return empty / no-match results for an unknown host, not an error.
func TestCrossInterface_UnknownHostReturnsEmpty(t *testing.T) {
	cfg := sharedConfig()
	tr := sharedMockTransport()

	mcpSrv := mcp.NewServer(cfg, tr)
	httpSrv := api.NewServer(cfg, tr)

	mcpText := callMCPStatus(t, mcpSrv, "ghost")
	if !strings.Contains(mcpText, "no hosts matched") {
		t.Errorf("MCP unknown host: expected 'no hosts matched', got:\n%s", mcpText)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status?host=ghost", nil)
	req.Header.Set("Authorization", "Bearer ci-test-key")
	w := httptest.NewRecorder()
	httpSrv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP unknown host: expected 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode HTTP response: %v", err)
	}

	// Accept either an empty hosts array or an explicit message field.
	hosts, hasHosts := body["hosts"].([]any)
	message, hasMessage := body["message"].(string)

	if hasHosts && len(hosts) == 0 {
		return // correct: empty list
	}
	if hasMessage && strings.Contains(message, "no hosts matched") {
		return // correct: explicit message
	}
	t.Errorf("HTTP unknown host: unexpected response body: %v", body)
}

// mapKeys returns the keys of a map for use in error messages.
func mapKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
