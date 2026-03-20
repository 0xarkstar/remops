package security

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// redirectTransport rewrites all requests to a test server base URL.
type redirectTransport struct {
	base string
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newURL := rt.base + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL += "?" + req.URL.RawQuery
	}

	var bodyReader io.Reader
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(body)
	}

	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, bodyReader)
	if err != nil {
		return nil, err
	}
	for k, vs := range req.Header {
		for _, v := range vs {
			newReq.Header.Add(k, v)
		}
	}
	return http.DefaultTransport.RoundTrip(newReq)
}

func newTestApprover(serverURL string) *TelegramApprover {
	ta := NewTelegramApprover("testtoken", "123456")
	ta.client = &http.Client{Transport: &redirectTransport{base: serverURL}}
	return ta
}

func TestNewTelegramApprover(t *testing.T) {
	ta := NewTelegramApprover("mytoken", "mychat")
	if ta.botToken != "mytoken" {
		t.Errorf("botToken: want mytoken, got %s", ta.botToken)
	}
	if ta.chatID != "mychat" {
		t.Errorf("chatID: want mychat, got %s", ta.chatID)
	}
	if ta.client == nil {
		t.Error("expected non-nil http client")
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid, err := generateUUID()
	if err != nil {
		t.Fatalf("generateUUID: %v", err)
	}
	if len(uuid) != 32 {
		t.Errorf("uuid length: want 32, got %d: %s", len(uuid), uuid)
	}
	uuid2, _ := generateUUID()
	if uuid == uuid2 {
		t.Error("two UUIDs should not be equal")
	}
}

func TestTelegramPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)
	var out struct{ OK bool `json:"ok"` }
	if err := ta.post("sendMessage", map[string]any{"chat_id": "123"}, &out); err != nil {
		t.Fatalf("post: %v", err)
	}
	if !out.OK {
		t.Error("expected ok=true")
	}
}

func TestSendApprovalMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": map[string]any{"message_id": 42},
		})
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)
	msgID, err := ta.sendApprovalMessage("docker restart app", "uuid123")
	if err != nil {
		t.Fatalf("sendApprovalMessage: %v", err)
	}
	if msgID != 42 {
		t.Errorf("message_id: want 42, got %d", msgID)
	}
}

func TestSendApprovalMessageNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"description": "Unauthorized",
		})
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)
	_, err := ta.sendApprovalMessage("action", "uuid")
	if err == nil {
		t.Fatal("expected error when ok=false")
	}
}

func TestEditMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)
	if err := ta.editMessage(42, "new text"); err != nil {
		t.Fatalf("editMessage: %v", err)
	}
}

func TestAnswerCallbackQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)
	if err := ta.answerCallbackQuery("cbq123"); err != nil {
		t.Fatalf("answerCallbackQuery: %v", err)
	}
}

func TestGetUpdates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": []map[string]any{
				{
					"update_id": 101,
					"callback_query": map[string]any{
						"id":   "cq1",
						"data": "approve:myuuid",
					},
				},
			},
		})
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)
	updates, nextOffset, err := ta.getUpdates(context.Background(), 0)
	if err != nil {
		t.Fatalf("getUpdates: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("want 1 update, got %d", len(updates))
	}
	if nextOffset != 102 {
		t.Errorf("nextOffset: want 102, got %d", nextOffset)
	}
	if updates[0].CallbackQuery == nil {
		t.Fatal("expected callback_query to be set")
	}
	if updates[0].CallbackQuery.Data != "approve:myuuid" {
		t.Errorf("callback data: want 'approve:myuuid', got %s", updates[0].CallbackQuery.Data)
	}
}

func TestGetUpdatesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	ta := newTestApprover(srv.URL)
	_, _, err := ta.getUpdates(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error for bad JSON response")
	}
}
