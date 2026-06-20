package cmd

import (
	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
)

// buildApprover constructs the Approver for the configured approval method.
// It returns (nil, false) when no usable approval channel is configured, so the
// caller can run without an out-of-band gate. Config validation already ensures
// the credentials a method needs are present; the checks here are defensive.
func buildApprover(a *config.ApprovalConfig) (security.Approver, bool) {
	if a == nil {
		return nil, false
	}

	switch a.Method {
	case "telegram":
		if token, chatID, ok := a.EffectiveTelegram(); ok {
			return security.NewTelegramApprover(token, chatID), true
		}
	case "discord":
		if token, channelID, ok := a.EffectiveDiscord(); ok {
			return security.NewDiscordApprover(token, channelID, a.Discord.AllowedUserIDs...), true
		}
	case "multi":
		// Mirror validateApproval: multi requires BOTH channels. Never silently
		// degrade a two-channel policy to one.
		tgToken, chatID, tgOK := a.EffectiveTelegram()
		dcToken, channelID, dcOK := a.EffectiveDiscord()
		if tgOK && dcOK {
			return security.NewMultiApprover(
				security.NewTelegramApprover(tgToken, chatID),
				security.NewDiscordApprover(dcToken, channelID, a.Discord.AllowedUserIDs...),
			), true
		}
	}

	return nil, false
}
