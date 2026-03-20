package security

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestApprovalSendFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "Unauthorized"})
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)
	_, err := ta.RequestApproval(context.Background(), "docker restart app")
	if err == nil {
		t.Fatal("expected error when sendMessage fails, got nil")
	}
}

func TestPollForCallbackCancelledContext(t *testing.T) {
	// Server returns empty updates (no matching callback).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := ta.pollForCallback(ctx, "someuuid")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestRequestApprovalApproved(t *testing.T) {
	var capturedUUID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case containsPath(r.URL.Path, "sendMessage"):
			// Parse body to get the callback_data prefix and extract UUID
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			// Return message_id 99
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 99},
			})

		case containsPath(r.URL.Path, "getUpdates"):
			if capturedUUID == "" {
				// First call: return empty (UUID not yet known)
				json.NewEncoder(w).Encode(map[string]any{
					"ok": true, "result": []any{},
				})
				return
			}
			// Return approval callback
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []map[string]any{
					{
						"update_id": 1,
						"callback_query": map[string]any{
							"id":   "cq1",
							"data": "approve:" + capturedUUID,
						},
					},
				},
			})

		case containsPath(r.URL.Path, "answerCallbackQuery"):
			json.NewEncoder(w).Encode(map[string]any{"ok": true})

		case containsPath(r.URL.Path, "editMessageText"):
			json.NewEncoder(w).Encode(map[string]any{"ok": true})

		default:
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)

	// Generate a UUID so we can inject it into the mock
	uuid, err := generateUUID()
	if err != nil {
		t.Fatal(err)
	}
	capturedUUID = uuid

	// Directly test pollForCallback with a server that returns the right callback
	approved, err := ta.pollForCallback(context.Background(), uuid)
	if err != nil {
		t.Fatalf("pollForCallback: %v", err)
	}
	if !approved {
		t.Error("expected approved=true")
	}
}

func TestPollForCallbackDenied(t *testing.T) {
	uuid, _ := generateUUID()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if containsPath(r.URL.Path, "getUpdates") {
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []map[string]any{
					{
						"update_id": 1,
						"callback_query": map[string]any{
							"id":   "cq1",
							"data": "deny:" + uuid,
						},
					},
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)
	approved, err := ta.pollForCallback(context.Background(), uuid)
	if err != nil {
		t.Fatalf("pollForCallback deny: %v", err)
	}
	if approved {
		t.Error("expected approved=false for deny callback")
	}
}

func containsPath(path, substr string) bool {
	return len(path) > 0 && (path == "/"+substr || len(path) > len(substr) && path[len(path)-len(substr)-1:] == "/"+substr)
}
