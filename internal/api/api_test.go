package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/transport"
)

// mockTransport implements transport.Transport for testing.
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

// minimalConfig returns a config with one host and one service, plus API key set.
func minimalConfig() *config.Config {
	return &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"web1": {Address: "1.1.1.1"}},
		Services: map[string]config.Service{
			"myapp": {Host: "web1", Container: "myapp_c"},
		},
		Profiles: map[string]config.Profile{},
		API:      &config.APIConfig{APIKey: "test-key", Listen: ":0"},
	}
}

// dbConfig returns a config whose "myapp" service has a PostgreSQL DB config.
func dbConfig() *config.Config {
	cfg := minimalConfig()
	cfg.Services["myapp"] = config.Service{
		Host:      "web1",
		Container: "myapp_c",
		DB: &config.DBConfig{
			Engine:   "postgresql",
			User:     "admin",
			Database: "mydb",
		},
	}
	return cfg
}

// newTestServer builds a Server with the minimal config and a mock transport.
func newTestServer(mt *mockTransport, opts ...ServerOption) *Server {
	return NewServer(minimalConfig(), mt, opts...)
}

// doRequest fires handler directly via httptest and returns the recorder.
func doRequest(handler http.HandlerFunc, method, target string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, body)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

// authHeader returns a map with a valid Authorization header.
func authHeader() map[string]string {
	return map[string]string{"Authorization": "Bearer test-key"}
}

// jsonBody encodes v as JSON and returns it as an io.Reader.
func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// decodeJSON decodes the recorder body into a map.
func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decodeJSON: %v (body=%q)", err, w.Body.String())
	}
	return m
}

// ---------------------------------------------------------------------------
// Auth middleware tests
// ---------------------------------------------------------------------------

func TestAuthMiddleware_NoHeader_Returns401(t *testing.T) {
	s := newTestServer(nil)
	w := doRequest(s.authMiddleware(s.handleVersion), "GET", "/api/v1/version", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidKey_Returns401(t *testing.T) {
	s := newTestServer(nil)
	w := doRequest(s.authMiddleware(s.handleVersion), "GET", "/api/v1/version", nil,
		map[string]string{"Authorization": "Bearer wrong-key"})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_MissingBearerPrefix_Returns401(t *testing.T) {
	s := newTestServer(nil)
	// Token sent without "Bearer " prefix — header value equals the raw key.
	w := doRequest(s.authMiddleware(s.handleVersion), "GET", "/api/v1/version", nil,
		map[string]string{"Authorization": "test-key"})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401 when Bearer prefix missing, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidKey_PassesThrough(t *testing.T) {
	s := newTestServer(nil)
	w := doRequest(s.authMiddleware(s.handleVersion), "GET", "/api/v1/version", nil, authHeader())
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_NoAPIConfig_Returns500(t *testing.T) {
	cfg := minimalConfig()
	cfg.API = nil
	s := NewServer(cfg, nil)
	w := doRequest(s.authMiddleware(s.handleVersion), "GET", "/api/v1/version", nil, authHeader())
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500 when API not configured, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/version
// ---------------------------------------------------------------------------

func TestVersion_Returns200WithVersionString(t *testing.T) {
	s := newTestServer(nil, WithVersion("1.2.3"))
	w := doRequest(s.authMiddleware(s.handleVersion), "GET", "/api/v1/version", nil, authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := decodeJSON(t, w)
	if m["version"] != "1.2.3" {
		t.Errorf("want version '1.2.3', got %v", m["version"])
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/status
// ---------------------------------------------------------------------------

func TestStatus_Success_Returns200WithHostData(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: `{"Names":"myapp_c","State":"running"}` + "\n"}, nil
		},
	}
	s := newTestServer(mt)
	w := doRequest(s.authMiddleware(s.handleStatus), "GET", "/api/v1/status", nil, authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := decodeJSON(t, w)
	hosts, ok := m["hosts"].([]any)
	if !ok || len(hosts) == 0 {
		t.Fatalf("want non-empty hosts array, got %v", m["hosts"])
	}
}

func TestStatus_FilterByHost_Returns200WithSingleHost(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, host, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "# " + host + "\n"}, nil
		},
	}
	s := newTestServer(mt)
	w := doRequest(s.authMiddleware(s.handleStatus), "GET", "/api/v1/status?host=web1", nil, authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := decodeJSON(t, w)
	hosts := m["hosts"].([]any)
	if len(hosts) != 1 {
		t.Errorf("want 1 host entry, got %d", len(hosts))
	}
}

func TestStatus_InvalidHostName_Returns400(t *testing.T) {
	s := newTestServer(nil)
	// A host name with a space is invalid per safeNameRe and safe to pass in a URL query.
	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	q := req.URL.Query()
	q.Set("host", "bad host name!")
	req.URL.RawQuery = q.Encode()
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStatus)(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid host name, got %d", w.Code)
	}
}

func TestStatus_UnknownHost_ReturnsEmptyHosts(t *testing.T) {
	s := newTestServer(nil)
	w := doRequest(s.authMiddleware(s.handleStatus), "GET", "/api/v1/status?host=notexist", nil, authHeader())
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := decodeJSON(t, w)
	if m["message"] != "no hosts matched" {
		t.Errorf("want 'no hosts matched', got %v", m["message"])
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/services/{name}/logs
// ---------------------------------------------------------------------------

func TestServiceLogs_Success_Returns200WithLogs(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "log line 1\nlog line 2\n"}, nil
		},
	}
	s := newTestServer(mt)
	req := httptest.NewRequest("GET", "/api/v1/services/myapp/logs", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceLogs)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := decodeJSON(t, w)
	logs, ok := m["logs"].(string)
	if !ok || !strings.Contains(logs, "log line") {
		t.Errorf("want logs containing 'log line', got %v", m["logs"])
	}
}

func TestServiceLogs_UnknownService_Returns404(t *testing.T) {
	s := newTestServer(nil)
	req := httptest.NewRequest("GET", "/api/v1/services/nope/logs", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "nope")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceLogs)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown service, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/services/{name}/restart
// ---------------------------------------------------------------------------

func TestServiceRestart_Success_Returns200(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0, Stdout: "myapp_c\n"}, nil
		},
	}
	s := newTestServer(mt)
	req := httptest.NewRequest("POST", "/api/v1/services/myapp/restart",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("restart"))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["action"] != "restart" {
		t.Errorf("want action 'restart', got %v", m["action"])
	}
}

func TestServiceRestart_NoConfirm_Returns400(t *testing.T) {
	s := newTestServer(nil)
	req := httptest.NewRequest("POST", "/api/v1/services/myapp/restart",
		jsonBody(map[string]bool{"confirm": false}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("restart"))(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 when confirm=false, got %d", w.Code)
	}
}

func TestServiceRestart_ViewerDenied_Returns403(t *testing.T) {
	s := newTestServer(nil, WithProfile(config.LevelViewer))
	req := httptest.NewRequest("POST", "/api/v1/services/myapp/restart",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("restart"))(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for viewer attempting restart, got %d", w.Code)
	}
}

func TestServiceRestart_UnknownService_Returns404(t *testing.T) {
	s := newTestServer(nil)
	req := httptest.NewRequest("POST", "/api/v1/services/nosvc/restart",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "nosvc")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("restart"))(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown service, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/hosts/{name}/disk
// ---------------------------------------------------------------------------

func TestHostDisk_Success_Returns200WithDiskData(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{
				Stdout: "Filesystem  Size  Used Avail Use%\n/dev/sda1   50G   10G   40G  20%\n",
			}, nil
		},
	}
	s := newTestServer(mt)
	req := httptest.NewRequest("GET", "/api/v1/hosts/web1/disk", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostDisk)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := decodeJSON(t, w)
	disk, ok := m["disk"].(string)
	if !ok || !strings.Contains(disk, "sda1") {
		t.Errorf("want disk output containing 'sda1', got %v", m["disk"])
	}
}

func TestHostDisk_UnknownHost_Returns404(t *testing.T) {
	s := newTestServer(nil)
	req := httptest.NewRequest("GET", "/api/v1/hosts/ghost/disk", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "ghost")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostDisk)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown host, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/hosts/{name}/exec
// ---------------------------------------------------------------------------

func TestHostExec_AdminRequired_Returns403ForViewer(t *testing.T) {
	s := newTestServer(nil, WithProfile(config.LevelViewer))
	req := httptest.NewRequest("POST", "/api/v1/hosts/web1/exec",
		jsonBody(map[string]any{"command": "echo hello", "confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostExec)(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for viewer attempting exec, got %d", w.Code)
	}
}

func TestHostExec_Success_Returns200WithOutput(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "hello\n", ExitCode: 0}, nil
		},
	}
	s := newTestServer(mt, WithProfile(config.LevelAdmin))
	req := httptest.NewRequest("POST", "/api/v1/hosts/web1/exec",
		jsonBody(map[string]any{"command": "echo hello", "confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostExec)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	output, ok := m["output"].(string)
	if !ok || !strings.Contains(output, "hello") {
		t.Errorf("want output containing 'hello', got %v", m["output"])
	}
}

func TestHostExec_NoConfirm_Returns400(t *testing.T) {
	s := newTestServer(nil, WithProfile(config.LevelAdmin))
	req := httptest.NewRequest("POST", "/api/v1/hosts/web1/exec",
		jsonBody(map[string]any{"command": "echo hello", "confirm": false}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostExec)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 when confirm=false, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/db/{service}/query
// ---------------------------------------------------------------------------

func TestDBQuery_Success_Returns200WithResult(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: " count \n-------\n     1\n"}, nil
		},
	}
	s := NewServer(dbConfig(), mt)
	req := httptest.NewRequest("POST", "/api/v1/db/myapp/query",
		jsonBody(map[string]string{"query": "SELECT 1"}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("service", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleDBQuery)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	result, ok := m["result"].(string)
	if !ok || !strings.Contains(result, "count") {
		t.Errorf("want result containing 'count', got %v", m["result"])
	}
}

func TestDBQuery_WriteQueryDenied_Returns403ForViewer(t *testing.T) {
	s := NewServer(dbConfig(), nil, WithProfile(config.LevelViewer))
	req := httptest.NewRequest("POST", "/api/v1/db/myapp/query",
		jsonBody(map[string]string{"query": "INSERT INTO t VALUES (1)"}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("service", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleDBQuery)(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for viewer write query, got %d", w.Code)
	}
}

func TestDBQuery_UnknownService_Returns404(t *testing.T) {
	s := NewServer(dbConfig(), nil)
	req := httptest.NewRequest("POST", "/api/v1/db/nosvc/query",
		jsonBody(map[string]string{"query": "SELECT 1"}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("service", "nosvc")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleDBQuery)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown service, got %d", w.Code)
	}
}

func TestDBQuery_ServiceWithNoDBConfig_Returns400(t *testing.T) {
	// minimalConfig's "myapp" has no DB config.
	s := newTestServer(nil)
	req := httptest.NewRequest("POST", "/api/v1/db/myapp/query",
		jsonBody(map[string]string{"query": "SELECT 1"}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("service", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleDBQuery)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 when service has no db config, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/doctor
// ---------------------------------------------------------------------------

func TestDoctor_Success_Returns200WithHostChecks(t *testing.T) {
	mt := &mockTransport{
		pingFunc: func(_ context.Context, host string) (transport.PingResult, error) {
			return transport.PingResult{Host: host, Online: true, Latency: 1_000_000}, nil
		},
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "24.0.0\n"}, nil
		},
	}
	s := newTestServer(mt)
	w := doRequest(s.authMiddleware(s.handleDoctor), "GET", "/api/v1/doctor", nil, authHeader())

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := decodeJSON(t, w)
	hosts, ok := m["hosts"].([]any)
	if !ok || len(hosts) == 0 {
		t.Errorf("want non-empty hosts array in doctor response, got %v", m["hosts"])
	}
	first := hosts[0].(map[string]any)
	if first["online"] != true {
		t.Errorf("want host online=true, got %v", first["online"])
	}
}

func TestDoctor_OfflineHost_ReportsUnreachable(t *testing.T) {
	mt := &mockTransport{
		pingFunc: func(_ context.Context, host string) (transport.PingResult, error) {
			return transport.PingResult{Host: host, Online: false}, nil
		},
	}
	s := newTestServer(mt)
	w := doRequest(s.authMiddleware(s.handleDoctor), "GET", "/api/v1/doctor", nil, authHeader())

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	m := decodeJSON(t, w)
	hosts := m["hosts"].([]any)
	first := hosts[0].(map[string]any)
	if first["online"] != false {
		t.Errorf("want host online=false for offline host, got %v", first["online"])
	}
}

// ---------------------------------------------------------------------------
// X-Remops-Profile header — profile override
// ---------------------------------------------------------------------------

func TestProfileHeader_OverridesServerDefault(t *testing.T) {
	// Server is admin by default, but request sends viewer profile header.
	// Restart requires operator — viewer should get 403.
	s := newTestServer(nil, WithProfile(config.LevelAdmin))
	req := httptest.NewRequest("POST", "/api/v1/services/myapp/restart",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Remops-Profile", "viewer")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("restart"))(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 when profile header downgrades to viewer, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// NewServer / ServerOption smoke tests
// ---------------------------------------------------------------------------

func TestNewServer_DefaultsToAdminProfile(t *testing.T) {
	s := NewServer(minimalConfig(), nil)
	if s.profileLevel != config.LevelAdmin {
		t.Errorf("want default profile admin, got %v", s.profileLevel)
	}
}

func TestWithVersion_SetsVersionOnServer(t *testing.T) {
	s := NewServer(minimalConfig(), nil, WithVersion("9.9.9"))
	if s.version != "9.9.9" {
		t.Errorf("WithVersion: want '9.9.9', got %q", s.version)
	}
}

func TestWithProfile_SetsProfileOnServer(t *testing.T) {
	s := NewServer(minimalConfig(), nil, WithProfile(config.LevelOperator))
	if s.profileLevel != config.LevelOperator {
		t.Errorf("WithProfile: want operator, got %v", s.profileLevel)
	}
}

// ---------------------------------------------------------------------------
// Stack helpers
// ---------------------------------------------------------------------------

func stackConfig() *config.Config {
	cfg := minimalConfig()
	cfg.Stacks = map[string]config.Stack{
		"monitoring": {Host: "web1", Path: "/home/user/monitoring"},
	}
	return cfg
}

// ---------------------------------------------------------------------------
// GET /api/v1/stacks/{name}/ps
// ---------------------------------------------------------------------------

func TestStackPS_API_Success(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: `[{"Name":"prometheus","State":"running"}]` + "\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(stackConfig(), mt)
	req := httptest.NewRequest("GET", "/api/v1/stacks/monitoring/ps", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackPS)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["stack"] != "monitoring" {
		t.Errorf("want stack 'monitoring', got %v", m["stack"])
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/stacks/{name}/up
// ---------------------------------------------------------------------------

func TestStackUp_API_Success(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "Container prometheus  Started\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(stackConfig(), mt)
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/up",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackAction("up -d"))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["stack"] != "monitoring" {
		t.Errorf("want stack 'monitoring', got %v", m["stack"])
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/stacks/{name}/down — viewer denied
// ---------------------------------------------------------------------------

func TestStackDown_API_ViewerDenied(t *testing.T) {
	s := NewServer(stackConfig(), nil, WithProfile(config.LevelViewer))
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/down",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackDown)(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for viewer attempting stack down, got %d", w.Code)
	}
}
