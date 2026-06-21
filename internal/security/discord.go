package security

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// Custom ID prefixes for the approval buttons. The full custom_id is
// "<action>:<nonce>" so concurrent approval requests don't collide.
const (
	discordApproveAction = "approve"
	discordDenyAction    = "deny"
)

// DiscordApprover implements Approver using Discord message-component buttons
// over a gateway (websocket) connection.
//
// It posts an approval message with ✅/❌ buttons, then blocks until an
// authorized user clicks one. Button interactions carry a Discord-verified user
// id, so — unlike reaction polling — there is no ambiguity about who acted, no
// pagination window, and no way for the bot's own action to be counted.
//
// The gateway connection is outbound only (no public inbound endpoint), so it
// works behind NAT/Tailscale on OCI. It is opened lazily on first use and kept
// open for the approver's lifetime.
//
// Security posture (fail closed): an unauthorized clicker is rejected (and the
// request keeps waiting); ctx cancel/timeout returns (false, err); when
// allowedUserIDs is non-empty only those users may decide.
type DiscordApprover struct {
	botToken       string
	channelID      string
	allowedUserIDs map[string]bool

	mu      sync.Mutex
	session *discordgo.Session
	pending map[string]chan Approval // nonce -> result channel
}

// NewDiscordApprover creates a DiscordApprover for the given bot token and
// channel. If allowedUserIDs are provided, only those users may approve/deny.
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
		pending:        make(map[string]chan Approval),
	}
}

// RequestApproval posts an approval message with ✅/❌ buttons and blocks until
// an authorized user clicks one or ctx is cancelled.
func (d *DiscordApprover) RequestApproval(ctx context.Context, action string) (Approval, error) {
	session, err := d.ensureSession()
	if err != nil {
		return Approval{}, err
	}

	nonce, err := generateUUID() // shared helper (telegram.go)
	if err != nil {
		return Approval{}, fmt.Errorf("approval nonce: %w", err)
	}

	resultCh := make(chan Approval, 1)
	d.mu.Lock()
	d.pending[nonce] = resultCh
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.pending, nonce)
		d.mu.Unlock()
	}()

	msg, err := session.ChannelMessageSendComplex(d.channelID, &discordgo.MessageSend{
		// Code-fence the action and disable all mentions so a crafted command
		// string cannot inject @everyone/role pings or active links.
		Content:    fmt.Sprintf("🔐 **Approval required**\n```\n%s\n```", action),
		Components: approvalButtons(nonce),
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{},
		},
	})
	if err != nil {
		return Approval{}, fmt.Errorf("send approval message: %w", err)
	}

	select {
	case approval := <-resultCh:
		return approval, nil
	case <-ctx.Done():
		// Distinguish "another channel handled it" from "timed out".
		d.disableButtons(session, msg.ID, cancellationMessage(ctx, action))
		return Approval{}, fmt.Errorf("approval timed out: %w", ctx.Err())
	}
}

// ensureSession lazily opens the gateway connection and registers the
// interaction handler. Safe for concurrent callers.
func (d *DiscordApprover) ensureSession() (*discordgo.Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.session != nil {
		return d.session, nil
	}

	s, err := discordgo.New("Bot " + d.botToken)
	if err != nil {
		return nil, fmt.Errorf("discord session: %w", err)
	}
	// Component interactions are delivered without privileged intents; Guilds
	// is the minimal scope needed for the gateway connection.
	s.Identify.Intents = discordgo.IntentsGuilds
	s.AddHandler(d.onInteraction)
	if err := s.Open(); err != nil {
		return nil, fmt.Errorf("discord gateway open: %w", err)
	}
	d.session = s
	return s, nil
}

// Close shuts down the gateway connection if one is open.
func (d *DiscordApprover) Close() error {
	d.mu.Lock()
	s := d.session
	d.session = nil
	d.mu.Unlock()
	if s != nil {
		return s.Close()
	}
	return nil
}

// onInteraction handles button clicks, validates the clicker, acknowledges the
// interaction, and resolves the matching pending request.
func (d *DiscordApprover) onInteraction(s *discordgo.Session, ic *discordgo.InteractionCreate) {
	if ic.Type != discordgo.InteractionMessageComponent {
		return
	}
	action, nonce, ok := parseCustomID(ic.MessageComponentData().CustomID)
	if !ok {
		return
	}

	d.mu.Lock()
	resultCh, known := d.pending[nonce]
	d.mu.Unlock()
	if !known {
		return // stale or unknown request
	}

	clicker := interactionUserID(ic)
	if !authorizedClicker(clicker, d.allowedUserIDs) {
		_ = s.InteractionRespond(ic.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "You are not authorized to approve this action.",
			},
		})
		return // do NOT resolve — keep waiting for an authorized decision
	}

	approved := action == discordApproveAction
	verdict := "✅ Approved"
	if !approved {
		verdict = "❌ Denied"
	}
	// Acknowledge by replacing the message and removing the buttons.
	_ = s.InteractionRespond(ic.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    fmt.Sprintf("%s by <@%s>", verdict, clicker),
			Components: []discordgo.MessageComponent{},
		},
	})

	d.mu.Lock()
	delete(d.pending, nonce)
	d.mu.Unlock()
	resultCh <- Approval{Approved: approved, By: clicker, Via: "discord"} // buffered; never blocks
}

// disableButtons edits a message to remove its buttons and set final text.
// Best-effort: errors are ignored (the request is already resolving).
func (d *DiscordApprover) disableButtons(s *discordgo.Session, msgID, content string) {
	empty := []discordgo.MessageComponent{}
	_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    d.channelID,
		ID:         msgID,
		Content:    &content,
		Components: &empty,
	})
}

// approvalButtons builds the ✅/❌ action row for a given nonce.
func approvalButtons(nonce string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "✅ Approve",
				Style:    discordgo.SuccessButton,
				CustomID: discordApproveAction + ":" + nonce,
			},
			discordgo.Button{
				Label:    "❌ Deny",
				Style:    discordgo.DangerButton,
				CustomID: discordDenyAction + ":" + nonce,
			},
		}},
	}
}

// parseCustomID splits "approve:<nonce>" / "deny:<nonce>" into its parts.
func parseCustomID(customID string) (action, nonce string, ok bool) {
	for _, a := range []string{discordApproveAction, discordDenyAction} {
		prefix := a + ":"
		if strings.HasPrefix(customID, prefix) {
			n := customID[len(prefix):]
			if n == "" {
				return "", "", false
			}
			return a, n, true
		}
	}
	return "", "", false
}

// authorizedClicker reports whether a clicker may decide: a non-empty id, and
// (when an allowlist is set) a member of it.
func authorizedClicker(clickerID string, allowed map[string]bool) bool {
	if clickerID == "" {
		return false
	}
	if len(allowed) == 0 {
		return true
	}
	return allowed[clickerID]
}

// interactionUserID extracts the acting user's id from a guild or DM interaction.
func interactionUserID(ic *discordgo.InteractionCreate) string {
	if ic.Member != nil && ic.Member.User != nil {
		return ic.Member.User.ID
	}
	if ic.User != nil {
		return ic.User.ID
	}
	return ""
}
