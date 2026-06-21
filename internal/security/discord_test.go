package security

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

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
	if d.pending == nil {
		t.Error("pending map must be initialized")
	}
}

func TestParseCustomID(t *testing.T) {
	cases := []struct {
		in          string
		wantAction  string
		wantNonce   string
		wantOK      bool
	}{
		{"approve:abc123", "approve", "abc123", true},
		{"deny:abc123", "deny", "abc123", true},
		{"approve:", "", "", false},   // empty nonce rejected
		{"deny:", "", "", false},
		{"approve", "", "", false},    // no separator
		{"other:abc", "", "", false},  // unknown action
		{"", "", "", false},
		{"approve:has:colon", "approve", "has:colon", true}, // nonce may contain colons
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			a, n, ok := parseCustomID(c.in)
			if a != c.wantAction || n != c.wantNonce || ok != c.wantOK {
				t.Errorf("parseCustomID(%q) = (%q,%q,%v), want (%q,%q,%v)",
					c.in, a, n, ok, c.wantAction, c.wantNonce, c.wantOK)
			}
		})
	}
}

func TestAuthorizedClicker(t *testing.T) {
	allow := map[string]bool{"op1": true, "op2": true}
	cases := []struct {
		name    string
		clicker string
		allowed map[string]bool
		want    bool
	}{
		{"empty clicker rejected", "", allow, false},
		{"empty clicker rejected (no allowlist)", "", nil, false},
		{"no allowlist allows any", "anyone", nil, true},
		{"no allowlist allows any (empty map)", "anyone", map[string]bool{}, true},
		{"allowlisted operator", "op1", allow, true},
		{"non-allowlisted rejected", "rando", allow, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := authorizedClicker(c.clicker, c.allowed); got != c.want {
				t.Errorf("authorizedClicker(%q, %v) = %v, want %v", c.clicker, c.allowed, got, c.want)
			}
		})
	}
}

func TestInteractionUserID(t *testing.T) {
	// Guild interaction: prefer Member.User.
	guild := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		Member: &discordgo.Member{User: &discordgo.User{ID: "guilduser"}},
	}}
	if got := interactionUserID(guild); got != "guilduser" {
		t.Errorf("guild: want guilduser, got %q", got)
	}
	// DM interaction: fall back to User.
	dm := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
		User: &discordgo.User{ID: "dmuser"},
	}}
	if got := interactionUserID(dm); got != "dmuser" {
		t.Errorf("dm: want dmuser, got %q", got)
	}
	// Neither set: empty (fails closed via authorizedClicker).
	empty := &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{}}
	if got := interactionUserID(empty); got != "" {
		t.Errorf("empty: want '', got %q", got)
	}
}

func TestApprovalButtons(t *testing.T) {
	comps := approvalButtons("nonce42")
	if len(comps) != 1 {
		t.Fatalf("want 1 action row, got %d", len(comps))
	}
	row, ok := comps[0].(discordgo.ActionsRow)
	if !ok {
		t.Fatalf("want ActionsRow, got %T", comps[0])
	}
	if len(row.Components) != 2 {
		t.Fatalf("want 2 buttons, got %d", len(row.Components))
	}
	approve, ok := row.Components[0].(discordgo.Button)
	if !ok {
		t.Fatalf("want Button, got %T", row.Components[0])
	}
	if approve.CustomID != "approve:nonce42" {
		t.Errorf("approve custom_id: want approve:nonce42, got %s", approve.CustomID)
	}
	if approve.Style != discordgo.SuccessButton {
		t.Errorf("approve style: want SuccessButton, got %v", approve.Style)
	}
	deny := row.Components[1].(discordgo.Button)
	if deny.CustomID != "deny:nonce42" {
		t.Errorf("deny custom_id: want deny:nonce42, got %s", deny.CustomID)
	}
	if deny.Style != discordgo.DangerButton {
		t.Errorf("deny style: want DangerButton, got %v", deny.Style)
	}
	// Round-trips through parseCustomID.
	if a, n, ok := parseCustomID(approve.CustomID); !ok || a != "approve" || n != "nonce42" {
		t.Errorf("approve custom_id does not round-trip: %q %q %v", a, n, ok)
	}
}
