package security

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestDiscordLiveButtonApproval drives the real DiscordApprover against a live
// bot + channel. It is skipped unless the approver credentials are present in
// the environment, so the normal test suite (and CI) never touches Discord:
//
//	set -a; . ~/.config/remops/fleet-approver.env; set +a
//	go test -run TestDiscordLiveButtonApproval -v ./internal/security/ -timeout 180s
//
// When run, it posts an approval message with buttons and blocks until an
// authorized user clicks one (or the 150s window elapses).
func TestDiscordLiveButtonApproval(t *testing.T) {
	token := os.Getenv("REMOPS_APPROVER_DISCORD_TOKEN")
	channel := os.Getenv("REMOPS_APPROVER_DISCORD_CHANNEL_ID")
	op := os.Getenv("REMOPS_APPROVER_DISCORD_OPERATOR_UID")
	if token == "" || channel == "" {
		t.Skip("set REMOPS_APPROVER_DISCORD_TOKEN and REMOPS_APPROVER_DISCORD_CHANNEL_ID to run the live test")
	}

	var allow []string
	if op != "" {
		allow = append(allow, op)
	}
	d := NewDiscordApprover(token, channel, allow...)
	defer func() { _ = d.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	t.Log("Posting approval message — click ✅ Approve or ❌ Deny in Discord…")
	approval, err := d.RequestApproval(ctx, "remops e2e button test (2026-06-21)")
	if err != nil {
		t.Fatalf("RequestApproval failed: %v", err)
	}
	t.Logf("LIVE RESULT: approved=%v by=%s via=%s", approval.Approved, approval.By, approval.Via)
}
