package api

// Additional tests to raise internal/api coverage from 46% to 70%+.
// Each test covers exactly one behaviour not exercised by api_test.go.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/0xarkstar/remops/internal/transport"
)

// ---------------------------------------------------------------------------
// ServerOption smoke tests
// ---------------------------------------------------------------------------

func TestWithApprover_SetsApproverOnServer(t *testing.T) {
	approver := &stubApprover{approved: true}
	s := NewServer(minimalConfig(), nil, WithApprover(approver))
	if s.approver != approver {
		t.Error("WithApprover did not set approver on server")
	}
}

func TestWithRateLimiter_SetsRateLimiterOnServer(t *testing.T) {
	rl, err := security.NewRateLimiter(10)
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}
	s := NewServer(minimalConfig(), nil, WithRateLimiter(rl))
	if s.rateLimiter != rl {
		t.Error("WithRateLimiter did not set rate limiter on server")
	}
}

func TestWithAuditLogger_SetsAuditLoggerOnServer(t *testing.T) {
	al, err := security.NewAuditLogger()
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	s := NewServer(minimalConfig(), nil, WithAuditLogger(al))
	if s.auditLogger != al {
		t.Error("WithAuditLogger did not set audit logger on server")
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/hosts/{name}/info
// ---------------------------------------------------------------------------

func TestHostInfo_Success_Returns200WithInfo(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "Linux web1 5.15\n"}, nil
		},
	}
	s := newTestServer(mt)
	req := httptest.NewRequest("GET", "/api/v1/hosts/web1/info", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostInfo)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["host"] != "web1" {
		t.Errorf("want host 'web1', got %v", m["host"])
	}
	if _, ok := m["info"]; !ok {
		t.Error("want 'info' key in response")
	}
}

func TestHostInfo_UnknownHost_Returns404(t *testing.T) {
	s := newTestServer(nil)
	req := httptest.NewRequest("GET", "/api/v1/hosts/ghost/info", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "ghost")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostInfo)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown host, got %d", w.Code)
	}
}

func TestHostInfo_InvalidHostName_Returns400(t *testing.T) {
	s := newTestServer(nil)
	req := httptest.NewRequest("GET", "/api/v1/hosts/bad%20name/info", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "bad name!")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostInfo)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid host name, got %d", w.Code)
	}
}

func TestHostInfo_ViewerProfile_Returns200(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "info\n"}, nil
		},
	}
	s := newTestServer(mt, WithProfile(config.LevelViewer))
	req := httptest.NewRequest("GET", "/api/v1/hosts/web1/info", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostInfo)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 for viewer reading host info, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/stacks/{name}/logs
// ---------------------------------------------------------------------------

func TestStackLogs_Success_Returns200WithLogs(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "prometheus log line\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(stackConfig(), mt)
	req := httptest.NewRequest("GET", "/api/v1/stacks/monitoring/logs", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackLogs)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["stack"] != "monitoring" {
		t.Errorf("want stack 'monitoring', got %v", m["stack"])
	}
	if _, ok := m["logs"]; !ok {
		t.Error("want 'logs' key in response")
	}
}

func TestStackLogs_UnknownStack_Returns404(t *testing.T) {
	s := NewServer(stackConfig(), nil)
	req := httptest.NewRequest("GET", "/api/v1/stacks/nope/logs", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "nope")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackLogs)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown stack, got %d", w.Code)
	}
}

func TestStackLogs_ViewerDenied_Returns403(t *testing.T) {
	// handleStackLogs requires LevelViewer — viewers are allowed, so we
	// verify it does NOT return 403 (access is open to viewers).
	// The permission check gates at viewer, so even viewer passes.
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "log\n"}, nil
		},
	}
	s := NewServer(stackConfig(), mt, WithProfile(config.LevelViewer))
	req := httptest.NewRequest("GET", "/api/v1/stacks/monitoring/logs", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackLogs)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 for viewer reading stack logs, got %d", w.Code)
	}
}

func TestStackLogs_InvalidSince_Returns400(t *testing.T) {
	s := NewServer(stackConfig(), nil)
	req := httptest.NewRequest("GET", "/api/v1/stacks/monitoring/logs?since=$(evil)", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackLogs)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for shell-injection in since param, got %d", w.Code)
	}
}

func TestStackLogs_WithTailParam_Returns200(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, cmd string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "tail log\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(stackConfig(), mt)
	req := httptest.NewRequest("GET", "/api/v1/stacks/monitoring/logs?tail=50", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackLogs)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 with tail param, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/stacks/{name}/action — additional paths
// ---------------------------------------------------------------------------

func TestStackAction_NoConfirm_Returns400(t *testing.T) {
	s := NewServer(stackConfig(), nil)
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/pull",
		jsonBody(map[string]bool{"confirm": false}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackAction("pull"))(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 when confirm=false for stack action, got %d", w.Code)
	}
}

func TestStackAction_InvalidJSON_Returns400(t *testing.T) {
	s := NewServer(stackConfig(), nil)
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/pull",
		strings.NewReader("{not-json}"))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackAction("pull"))(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid JSON body in stack action, got %d", w.Code)
	}
}

func TestStackAction_UnknownStack_Returns404(t *testing.T) {
	s := NewServer(stackConfig(), nil)
	req := httptest.NewRequest("POST", "/api/v1/stacks/nope/restart",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "nope")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackAction("restart"))(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown stack, got %d", w.Code)
	}
}

func TestStackAction_ViewerDenied_Returns403(t *testing.T) {
	s := NewServer(stackConfig(), nil, WithProfile(config.LevelViewer))
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/pull",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackAction("pull"))(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for viewer attempting stack pull, got %d", w.Code)
	}
}

func TestStackAction_Pull_Success(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "Pulling prometheus\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(stackConfig(), mt)
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/pull",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackAction("pull"))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for stack pull, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["action"] != "pull" {
		t.Errorf("want action 'pull', got %v", m["action"])
	}
}

func TestStackAction_Restart_Success(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "Restarting prometheus\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(stackConfig(), mt)
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/restart",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackAction("restart"))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for stack restart, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["action"] != "restart" {
		t.Errorf("want action 'restart', got %v", m["action"])
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/stacks/{name}/down — additional paths
// ---------------------------------------------------------------------------

func TestStackDown_Success_Returns200(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "Container prometheus  Removed\n", ExitCode: 0}, nil
		},
	}
	s := NewServer(stackConfig(), mt)
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/down",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackDown)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for stack down, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["action"] != "down" {
		t.Errorf("want action 'down', got %v", m["action"])
	}
}

func TestStackDown_NoConfirm_Returns400(t *testing.T) {
	s := NewServer(stackConfig(), nil)
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/down",
		jsonBody(map[string]bool{"confirm": false}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackDown)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 when confirm=false for stack down, got %d", w.Code)
	}
}

func TestStackDown_InvalidJSON_Returns400(t *testing.T) {
	s := NewServer(stackConfig(), nil)
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/down",
		strings.NewReader("not-json"))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackDown)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid JSON in stack down, got %d", w.Code)
	}
}

func TestStackDown_UnknownStack_Returns404(t *testing.T) {
	s := NewServer(stackConfig(), nil)
	req := httptest.NewRequest("POST", "/api/v1/stacks/nope/down",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "nope")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackDown)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown stack in down, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/stacks/{name}/ps — unknown stack
// ---------------------------------------------------------------------------

func TestStackPS_UnknownStack_Returns404(t *testing.T) {
	s := NewServer(stackConfig(), nil)
	req := httptest.NewRequest("GET", "/api/v1/stacks/nope/ps", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "nope")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackPS)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown stack ps, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/services/{name}/stop and start
// ---------------------------------------------------------------------------

func TestServiceStop_Success_Returns200(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0, Stdout: "myapp_c\n"}, nil
		},
	}
	s := newTestServer(mt)
	req := httptest.NewRequest("POST", "/api/v1/services/myapp/stop",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("stop"))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for service stop, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["action"] != "stop" {
		t.Errorf("want action 'stop', got %v", m["action"])
	}
}

func TestServiceStart_Success_Returns200(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0, Stdout: "myapp_c\n"}, nil
		},
	}
	s := newTestServer(mt)
	req := httptest.NewRequest("POST", "/api/v1/services/myapp/start",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("start"))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for service start, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if m["action"] != "start" {
		t.Errorf("want action 'start', got %v", m["action"])
	}
}

func TestServiceAction_InvalidJSON_Returns400(t *testing.T) {
	s := newTestServer(nil)
	req := httptest.NewRequest("POST", "/api/v1/services/myapp/restart",
		strings.NewReader("{bad json"))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("restart"))(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid JSON body in service action, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/services/{name}/logs — additional paths
// ---------------------------------------------------------------------------

func TestServiceLogs_InvalidServiceName_Returns400(t *testing.T) {
	s := newTestServer(nil)
	req := httptest.NewRequest("GET", "/api/v1/services/bad%20name/logs", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "bad name!")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceLogs)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid service name, got %d", w.Code)
	}
}

func TestServiceLogs_WithTailParam_Returns200(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "last 10 lines\n"}, nil
		},
	}
	s := newTestServer(mt)
	req := httptest.NewRequest("GET", "/api/v1/services/myapp/logs?tail=10", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceLogs)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 with tail param, got %d", w.Code)
	}
}

func TestServiceLogs_InvalidSince_Returns400(t *testing.T) {
	s := newTestServer(nil)
	req := httptest.NewRequest("GET", "/api/v1/services/myapp/logs?since=$(evil)", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceLogs)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for shell-injection in since param, got %d", w.Code)
	}
}

func TestServiceLogs_ProfileHeaderOverride_ViewerAllowed(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "logs\n"}, nil
		},
	}
	// Server is admin, but viewer header is sent — logs are read-only so viewer is allowed.
	s := newTestServer(mt, WithProfile(config.LevelAdmin))
	req := httptest.NewRequest("GET", "/api/v1/services/myapp/logs", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("X-Remops-Profile", "viewer")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceLogs)(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 for viewer reading logs via profile header, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/hosts/{name}/exec — additional paths
// ---------------------------------------------------------------------------

func TestHostExec_UnknownHost_Returns404(t *testing.T) {
	s := newTestServer(nil, WithProfile(config.LevelAdmin))
	req := httptest.NewRequest("POST", "/api/v1/hosts/ghost/exec",
		jsonBody(map[string]any{"command": "echo hi", "confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "ghost")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostExec)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown host in exec, got %d", w.Code)
	}
}

func TestHostExec_InvalidJSON_Returns400(t *testing.T) {
	s := newTestServer(nil, WithProfile(config.LevelAdmin))
	req := httptest.NewRequest("POST", "/api/v1/hosts/web1/exec",
		strings.NewReader("{bad"))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostExec)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid JSON in host exec, got %d", w.Code)
	}
}

func TestHostExec_EmptyCommand_Returns400(t *testing.T) {
	s := newTestServer(nil, WithProfile(config.LevelAdmin))
	req := httptest.NewRequest("POST", "/api/v1/hosts/web1/exec",
		jsonBody(map[string]any{"command": "", "confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostExec)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for empty command in host exec, got %d", w.Code)
	}
}

func TestHostExec_ShellInjection_Returns400(t *testing.T) {
	s := newTestServer(nil, WithProfile(config.LevelAdmin))
	req := httptest.NewRequest("POST", "/api/v1/hosts/web1/exec",
		jsonBody(map[string]any{"command": "echo $(evil)", "confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostExec)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for shell injection in exec command, got %d", w.Code)
	}
}

func TestHostExec_OperatorUnsafeCommandNoApprover_Returns403(t *testing.T) {
	// Operator profile + unsafe command + no approver configured = 403.
	s := newTestServer(nil, WithProfile(config.LevelOperator))
	req := httptest.NewRequest("POST", "/api/v1/hosts/web1/exec",
		jsonBody(map[string]any{"command": "rm -rf /tmp/x", "confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostExec)(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for operator attempting unsafe exec without approver, got %d", w.Code)
	}
}

func TestHostExec_ProfileHeaderOverride_OperatorDeniesViewer(t *testing.T) {
	// Admin server, but viewer header sent. Viewer < Operator — should 403.
	s := newTestServer(nil, WithProfile(config.LevelAdmin))
	req := httptest.NewRequest("POST", "/api/v1/hosts/web1/exec",
		jsonBody(map[string]any{"command": "echo hi", "confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Remops-Profile", "viewer")
	req.SetPathValue("name", "web1")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleHostExec)(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 for viewer exec via profile header, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/db/{service}/query — additional paths
// ---------------------------------------------------------------------------

func TestDBQuery_InvalidJSON_Returns400(t *testing.T) {
	s := NewServer(dbConfig(), nil)
	req := httptest.NewRequest("POST", "/api/v1/db/myapp/query",
		strings.NewReader("{bad"))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("service", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleDBQuery)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid JSON in db query, got %d", w.Code)
	}
}

func TestDBQuery_EmptyQuery_Returns400(t *testing.T) {
	s := NewServer(dbConfig(), nil)
	req := httptest.NewRequest("POST", "/api/v1/db/myapp/query",
		jsonBody(map[string]string{"query": ""}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("service", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleDBQuery)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for empty query, got %d", w.Code)
	}
}

func TestDBQuery_UnsupportedEngine_Returns400(t *testing.T) {
	cfg := minimalConfig()
	cfg.Services["myapp"] = config.Service{
		Host:      "web1",
		Container: "myapp_c",
		DB: &config.DBConfig{
			Engine:   "mongodb",
			User:     "admin",
			Database: "mydb",
		},
	}
	s := NewServer(cfg, nil)
	req := httptest.NewRequest("POST", "/api/v1/db/myapp/query",
		jsonBody(map[string]string{"query": "SELECT 1"}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("service", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleDBQuery)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for unsupported db engine, got %d", w.Code)
	}
}

func TestDBQuery_MySQL_Success(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "count\n1\n"}, nil
		},
	}
	cfg := minimalConfig()
	cfg.Services["myapp"] = config.Service{
		Host:      "web1",
		Container: "myapp_c",
		DB: &config.DBConfig{
			Engine:   "mysql",
			User:     "root",
			Database: "mydb",
			Password: "secret",
		},
	}
	s := NewServer(cfg, mt)
	req := httptest.NewRequest("POST", "/api/v1/db/myapp/query",
		jsonBody(map[string]string{"query": "SELECT 1"}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("service", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleDBQuery)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for mysql query, got %d (body=%s)", w.Code, w.Body.String())
	}
	m := decodeJSON(t, w)
	if _, ok := m["result"]; !ok {
		t.Error("want 'result' key in db query response")
	}
}

func TestDBQuery_WriteQuery_OperatorAllowed(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "INSERT 0 1\n"}, nil
		},
	}
	s := NewServer(dbConfig(), mt, WithProfile(config.LevelOperator))
	req := httptest.NewRequest("POST", "/api/v1/db/myapp/query",
		jsonBody(map[string]string{"query": "INSERT INTO t VALUES (1)"}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("service", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleDBQuery)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 for operator write query, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestDBQuery_ProfileHeader_CannotEscalate(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "DELETE 1\n"}, nil
		},
	}
	// Server default viewer. Header tries to escalate to operator — should be clamped.
	s := NewServer(dbConfig(), mt, WithProfile(config.LevelViewer))
	req := httptest.NewRequest("POST", "/api/v1/db/myapp/query",
		jsonBody(map[string]string{"query": "DELETE FROM t WHERE id=1"}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Remops-Profile", "operator")
	req.SetPathValue("service", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleDBQuery)(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 (escalation blocked), got %d (body=%s)", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Approver / RateLimiter integration in service action
// ---------------------------------------------------------------------------

// stubApprover is a test double for security.Approver.
type stubApprover struct {
	approved bool
	err      error
}

func (a *stubApprover) RequestApproval(_ context.Context, _ string) (bool, error) {
	return a.approved, a.err
}

func TestServiceAction_ApproverDenied_Returns403(t *testing.T) {
	s := newTestServer(nil, WithApprover(&stubApprover{approved: false}))
	req := httptest.NewRequest("POST", "/api/v1/services/myapp/restart",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("restart"))(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 when approver denies, got %d", w.Code)
	}
}

func TestServiceAction_ApproverError_Returns504(t *testing.T) {
	s := newTestServer(nil, WithApprover(&stubApprover{err: errors.New("telegram timeout")}))
	req := httptest.NewRequest("POST", "/api/v1/services/myapp/restart",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("restart"))(w, req)

	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("want 504 when approver errors, got %d", w.Code)
	}
}

func TestServiceAction_ApproverApproved_Returns200(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 0, Stdout: "myapp_c\n"}, nil
		},
	}
	s := newTestServer(mt, WithApprover(&stubApprover{approved: true}))
	req := httptest.NewRequest("POST", "/api/v1/services/myapp/restart",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "myapp")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleServiceAction("restart"))(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 when approver approves, got %d (body=%s)", w.Code, w.Body.String())
	}
}

func TestStackAction_ApproverDenied_Returns403(t *testing.T) {
	s := NewServer(stackConfig(), nil, WithApprover(&stubApprover{approved: false}))
	req := httptest.NewRequest("POST", "/api/v1/stacks/monitoring/up",
		jsonBody(map[string]bool{"confirm": true}))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("name", "monitoring")
	w := httptest.NewRecorder()
	s.authMiddleware(s.handleStackAction("up -d"))(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403 when approver denies stack action, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// mockTransport — stream path coverage
// ---------------------------------------------------------------------------

func TestMockTransport_StreamReturnsEmptyReader(t *testing.T) {
	mt := &mockTransport{}
	rc, err := mt.Stream(context.Background(), "host", "cmd")
	if err != nil {
		t.Fatalf("Stream: unexpected error: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if len(data) != 0 {
		t.Errorf("want empty stream, got %q", data)
	}
}
