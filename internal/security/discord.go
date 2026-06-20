package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const discordAPIBase = "https://discord.com/api/v10"

// Approval reaction emojis. The bot seeds both so the operator can approve or
// deny with a single tap; a reaction by an authorized non-bot user decides.
const (
	discordApproveEmoji = "✅"
	discordDenyEmoji    = "❌"
)

// maxErrBodyLen bounds how much of an upstream response body is echoed into an
// error, so a large or sensitive body cannot flood logs or the caller.
const maxErrBodyLen = 256

// DiscordApprover implements Approver using the Discord Bot REST API.
//
// It posts an approval message to a channel, seeds ✅/❌ reactions, and polls
// the message's reactions until an authorized non-bot user reacts. This mirrors
// the Telegram getUpdates polling model — no public webhook endpoint or gateway
// connection is required.
//
// Security posture (fail closed):
//   - The bot's own identity is resolved authoritatively via GET /users/@me
//     (the same identity that performs the @me seed reaction), so the seeded
//     reaction is never mistaken for an operator's.
//   - When allowedUserIDs is non-empty, only those user ids may approve/deny;
//     otherwise any non-bot human reaction decides (intended for a private,
//     operator-only channel).
//   - Deny is evaluated before approve, and an approve is honored only when the
//     deny lookup also succeeded — a failing deny lookup can never let an
//     approve through.
type DiscordApprover struct {
	botToken       string
	channelID      string
	allowedUserIDs map[string]bool
	client         *http.Client
}

// NewDiscordApprover creates a DiscordApprover for the given bot token and
// channel. If allowedUserIDs are provided, only reactions from those user ids
// count as operator decisions.
func NewDiscordApprover(botToken, channelID string, allowedUserIDs ...string) *DiscordApprover {
	allow := make(map[string]bool, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		if id != "" {
			allow[id] = true
		}
	}
	return &DiscordApprover{
		botToken:       botToken,
		channelID:      channelID,
		allowedUserIDs: allow,
		client:         &http.Client{Timeout: 35 * time.Second},
	}
}

// RequestApproval posts an approval message with ✅/❌ reactions and blocks
// until an authorized non-bot user reacts or ctx is cancelled.
func (d *DiscordApprover) RequestApproval(ctx context.Context, action string) (bool, error) {
	// Resolve the bot's own user id from the authenticated token — the SAME
	// identity that seeds the @me reactions. Fail closed if it cannot be
	// determined, otherwise the bot's own ✅ could be read as approval.
	botUserID, err := d.botUserID(ctx)
	if err != nil {
		return false, fmt.Errorf("discord approval: resolve bot identity: %w", err)
	}
	if botUserID == "" {
		return false, fmt.Errorf("discord approval: empty bot identity; failing closed")
	}

	msgID, err := d.sendApprovalMessage(ctx, action)
	if err != nil {
		return false, fmt.Errorf("send approval message: %w", err)
	}

	if err := d.addReaction(ctx, msgID, discordApproveEmoji); err != nil {
		return false, fmt.Errorf("seed approve reaction: %w", err)
	}
	if err := d.addReaction(ctx, msgID, discordDenyEmoji); err != nil {
		return false, fmt.Errorf("seed deny reaction: %w", err)
	}

	approved, err := d.pollForReaction(ctx, msgID, botUserID)
	if err != nil {
		// ctx is likely cancelled here; edit with a fresh context.
		_ = d.editMessage(context.Background(), msgID, fmt.Sprintf("⏰ Expired: %s", action))
		return false, err
	}

	status := "✅ Approved"
	if !approved {
		status = "❌ Denied"
	}
	_ = d.editMessage(context.Background(), msgID, fmt.Sprintf("%s: %s", status, action))
	return approved, nil
}

// botUserID returns the bot account's own user id via GET /users/@me.
func (d *DiscordApprover) botUserID(ctx context.Context) (string, error) {
	var me struct {
		ID string `json:"id"`
	}
	if err := d.do(ctx, http.MethodGet, "/users/@me", nil, &me); err != nil {
		return "", err
	}
	return me.ID, nil
}

// sendApprovalMessage posts the approval prompt and returns the message id.
func (d *DiscordApprover) sendApprovalMessage(ctx context.Context, action string) (string, error) {
	payload := map[string]any{
		"content": fmt.Sprintf("🔐 Approval required\n\nAction: %s\n\nReact ✅ to approve or ❌ to deny.", action),
	}
	var resp struct {
		ID string `json:"id"`
	}
	path := fmt.Sprintf("/channels/%s/messages", d.channelID)
	if err := d.do(ctx, http.MethodPost, path, payload, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// addReaction seeds a bot reaction on the message so the operator can tap it.
func (d *DiscordApprover) addReaction(ctx context.Context, msgID, emoji string) error {
	path := fmt.Sprintf("/channels/%s/messages/%s/reactions/%s/@me",
		d.channelID, msgID, url.PathEscape(emoji))
	return d.do(ctx, http.MethodPut, path, nil, nil)
}

type discordUser struct {
	ID  string `json:"id"`
	Bot bool   `json:"bot"`
}

// getReactionUsers returns the users who reacted with emoji (first page, up to 100).
func (d *DiscordApprover) getReactionUsers(ctx context.Context, msgID, emoji string) ([]discordUser, error) {
	path := fmt.Sprintf("/channels/%s/messages/%s/reactions/%s?limit=100",
		d.channelID, msgID, url.PathEscape(emoji))
	var users []discordUser
	if err := d.do(ctx, http.MethodGet, path, nil, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// pollForReaction polls reaction state every 2s until an authorized human reacts.
//
// Deny is evaluated before approve so a deny dominates a simultaneous tie, and
// an approve is honored only when the deny lookup ALSO succeeded — a failing or
// suppressed deny lookup can never let an approve through.
func (d *DiscordApprover) pollForReaction(ctx context.Context, msgID, botUserID string) (bool, error) {
	for {
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("approval timed out: %w", ctx.Err())
		default:
		}

		denyUsers, denyErr := d.getReactionUsers(ctx, msgID, discordDenyEmoji)
		if denyErr == nil && d.reactedByOperator(denyUsers, botUserID) {
			return false, nil
		}
		approveUsers, approveErr := d.getReactionUsers(ctx, msgID, discordApproveEmoji)
		if denyErr == nil && approveErr == nil && d.reactedByOperator(approveUsers, botUserID) {
			return true, nil
		}

		select {
		case <-ctx.Done():
			return false, fmt.Errorf("approval timed out: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

// reactedByOperator reports whether any reacting user is an authorized operator:
// not the bot account, not flagged as a bot, and (when an allowlist is set) in it.
func (d *DiscordApprover) reactedByOperator(users []discordUser, botUserID string) bool {
	for _, u := range users {
		if u.ID == botUserID || u.Bot {
			continue
		}
		if len(d.allowedUserIDs) > 0 && !d.allowedUserIDs[u.ID] {
			continue
		}
		return true
	}
	return false
}

// editMessage replaces the content of a previously posted message.
func (d *DiscordApprover) editMessage(ctx context.Context, msgID, content string) error {
	path := fmt.Sprintf("/channels/%s/messages/%s", d.channelID, msgID)
	return d.do(ctx, http.MethodPatch, path, map[string]any{"content": content}, nil)
}

// do executes a Discord REST request with bot authentication. A non-2xx
// response is returned as an error with a bounded snippet of the body. out,
// when non-nil, receives the decoded JSON body.
func (d *DiscordApprover) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal %s body: %w", path, err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, discordAPIBase+path, reader)
	if err != nil {
		return fmt.Errorf("build %s request: %w", path, err)
	}
	req.Header.Set("Authorization", "Bot "+d.botToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s response: %w", path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord %s %s: status %d: %s", method, path, resp.StatusCode, truncateForError(respBody))
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode %s response: %w", path, err)
		}
	}
	return nil
}

// truncateForError bounds an upstream body to a short snippet for safe inclusion
// in error messages.
func truncateForError(b []byte) string {
	if len(b) <= maxErrBodyLen {
		return string(b)
	}
	return string(b[:maxErrBodyLen]) + "…(truncated)"
}
