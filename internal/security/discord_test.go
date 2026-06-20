package security

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestDiscordApprover(serverURL string, allowed ...string) *DiscordApprover {
	d := NewDiscordApprover("testtoken", "chan123", allowed...)
	d.client = &http.Client{Transport: &redirectTransport{base: serverURL}}
	return d
}

func TestNewDiscordApprover(t *testing.T) {
	d := NewDiscordApprover("mytoken", "mychan", "op1", "")
	if d.botToken != "mytoken" {
		t.Errorf("botToken: want mytoken, got %s", d.botToken)
	}
	if d.channelID != "mychan" {
		t.Errorf("channelID: want mychan, got %s", d.channelID)
	}
	if !d.allowedUserIDs["op1"] {
		t.Error("expected op1 in allowlist")
	}
	if d.allowedUserIDs[""] {
		t.Error("empty id must not be added to allowlist")
	}
	if d.client == nil {
		t.Error("expected non-nil http client")
	}
}

func TestDiscordSendApprovalMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bot testtoken" {
			t.Errorf("auth header: want 'Bot testtoken', got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "msg1"})
	}))
	defer srv.Close()

	d := newTestDiscordApprover(srv.URL)
	msgID, err := d.sendApprovalMessage(context.Background(), "docker restart app")
	if err != nil {
		t.Fatalf("sendApprovalMessage: %v", err)
	}
	if msgID != "msg1" {
		t.Errorf("msgID: want msg1, got %s", msgID)
	}
}

func TestDiscordSendApprovalMessageNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"401: Unauthorized","code":0}`))
	}))
	defer srv.Close()

	d := newTestDiscordApprover(srv.URL)
	_, err := d.sendApprovalMessage(context.Background(), "action")
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}

func TestDiscordReactedByOperator(t *testing.T) {
	noAllow := NewDiscordApprover("t", "c")
	withAllow := NewDiscordApprover("t", "c", "op1")

	cases := []struct {
		name  string
		d     *DiscordApprover
		users []discordUser
		want  bool
	}{
		{"only bot self", noAllow, []discordUser{{ID: "bot1"}}, false},
		{"only flagged bot", noAllow, []discordUser{{ID: "other", Bot: true}}, false},
		{"a human present, no allowlist", noAllow, []discordUser{{ID: "bot1"}, {ID: "human1"}}, true},
		{"empty", noAllow, nil, false},
		{"allowlisted operator", withAllow, []discordUser{{ID: "op1"}}, true},
		{"non-allowlisted human rejected", withAllow, []discordUser{{ID: "rando"}}, false},
		{"bot excluded even if in allowlist scan", withAllow, []discordUser{{ID: "bot1"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.d.reactedByOperator(c.users, "bot1"); got != c.want {
				t.Errorf("reactedByOperator(%v) = %v, want %v", c.users, got, c.want)
			}
		})
	}
}

// discordFlowServer routes the full approval flow. denyStatus, when non-zero,
// makes the deny-reaction GET fail with that status (to test deny-suppression).
func discordFlowServer(t *testing.T, approveUsers, denyUsers []discordUser, denyStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/users/@me"):
			json.NewEncoder(w).Encode(map[string]any{"id": "bot1"})
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/messages"):
			json.NewEncoder(w).Encode(map[string]any{"id": "msg1"})
		case r.Method == http.MethodPut && strings.Contains(path, "/reactions/"):
			w.WriteHeader(http.StatusNoContent) // seed reaction
		case r.Method == http.MethodGet && strings.Contains(path, "/reactions/"):
			if strings.Contains(path, discordDenyEmoji) {
				if denyStatus != 0 {
					w.WriteHeader(denyStatus)
					w.Write([]byte(`{"message":"rate limited"}`))
					return
				}
				json.NewEncoder(w).Encode(denyUsers)
			} else {
				json.NewEncoder(w).Encode(approveUsers)
			}
		case r.Method == http.MethodPatch && strings.Contains(path, "/messages/"):
			json.NewEncoder(w).Encode(map[string]any{"id": "msg1"})
		default:
			t.Errorf("unexpected request %s %s", r.Method, path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
}

func TestDiscordRequestApprovalApproved(t *testing.T) {
	srv := discordFlowServer(t,
		[]discordUser{{ID: "bot1"}, {ID: "human1"}}, // approve: a human reacted
		[]discordUser{{ID: "bot1"}},                 // deny: only the bot
		0,
	)
	defer srv.Close()

	d := newTestDiscordApprover(srv.URL)
	approved, err := d.RequestApproval(context.Background(), "docker restart app")
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if !approved {
		t.Error("expected approved=true")
	}
}

func TestDiscordRequestApprovalDenied(t *testing.T) {
	srv := discordFlowServer(t,
		[]discordUser{{ID: "bot1"}, {ID: "human1"}}, // approve also has a human...
		[]discordUser{{ID: "bot1"}, {ID: "human1"}}, // ...but deny does too -> deny dominates
		0,
	)
	defer srv.Close()

	d := newTestDiscordApprover(srv.URL)
	approved, err := d.RequestApproval(context.Background(), "docker restart app")
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if approved {
		t.Error("expected approved=false: deny must dominate a simultaneous tie")
	}
}

func TestDiscordRequestApprovalEmptyBotID(t *testing.T) {
	// GET /users/@me returns no id → must fail closed, never auto-approve.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{}) // no id
	}))
	defer srv.Close()

	d := newTestDiscordApprover(srv.URL)
	approved, err := d.RequestApproval(context.Background(), "action")
	if err == nil {
		t.Fatal("expected fail-closed error when bot id unknown")
	}
	if approved {
		t.Error("must not approve when bot id is unknown")
	}
}

func TestDiscordDenySuppressionDoesNotApprove(t *testing.T) {
	// Deny lookup fails while approve shows a human. The approve must NOT be
	// honored; the gate stays waiting until the context times out (deny).
	srv := discordFlowServer(t,
		[]discordUser{{ID: "bot1"}, {ID: "human1"}}, // approve has a human
		nil,
		http.StatusTooManyRequests, // deny lookup errors
	)
	defer srv.Close()

	d := newTestDiscordApprover(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	approved, err := d.pollForReaction(ctx, "msg1", "bot1")
	if approved {
		t.Fatal("must not approve while the deny lookup is failing (deny-suppression)")
	}
	if err == nil {
		t.Fatal("expected timeout error, not a decision")
	}
}

func TestDiscordPollForReactionCancelled(t *testing.T) {
	srv := discordFlowServer(t,
		[]discordUser{{ID: "bot1"}}, // no human ever reacts
		[]discordUser{{ID: "bot1"}},
		0,
	)
	defer srv.Close()

	d := newTestDiscordApprover(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := d.pollForReaction(ctx, "msg1", "bot1")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDiscordErrorBodyTruncatedAndTokenFree(t *testing.T) {
	big := strings.Repeat("A", 5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(big))
	}))
	defer srv.Close()

	d := newTestDiscordApprover(srv.URL)
	_, err := d.sendApprovalMessage(context.Background(), "action")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "testtoken") {
		t.Error("bot token must never appear in an error")
	}
	if len(err.Error()) > maxErrBodyLen+200 {
		t.Errorf("error body should be truncated, got len %d", len(err.Error()))
	}
}

func TestTruncateForError(t *testing.T) {
	short := []byte("ok")
	if truncateForError(short) != "ok" {
		t.Error("short body should pass through unchanged")
	}
	long := []byte(strings.Repeat("x", maxErrBodyLen+50))
	out := truncateForError(long)
	if !strings.HasSuffix(out, "…(truncated)") {
		t.Error("long body should be marked truncated")
	}
	if len(out) > maxErrBodyLen+len("…(truncated)") {
		t.Error("truncated body exceeds bound")
	}
}
