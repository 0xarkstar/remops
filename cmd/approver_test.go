package cmd

import (
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
)

func TestBuildApprover(t *testing.T) {
	tg := &config.TelegramApprovalConfig{BotToken: "t", ChatID: "c"}
	dc := &config.DiscordApprovalConfig{BotToken: "d", ChannelID: "ch"}

	t.Run("nil config", func(t *testing.T) {
		if _, ok := buildApprover(nil); ok {
			t.Error("want ok=false for nil config")
		}
	})

	t.Run("telegram", func(t *testing.T) {
		a, ok := buildApprover(&config.ApprovalConfig{Method: "telegram", BotToken: "t", ChatID: "c"})
		if !ok {
			t.Fatal("want ok=true")
		}
		if _, isTG := a.(*security.TelegramApprover); !isTG {
			t.Errorf("want *TelegramApprover, got %T", a)
		}
	})

	t.Run("discord", func(t *testing.T) {
		a, ok := buildApprover(&config.ApprovalConfig{Method: "discord", Discord: dc})
		if !ok {
			t.Fatal("want ok=true")
		}
		if _, isDC := a.(*security.DiscordApprover); !isDC {
			t.Errorf("want *DiscordApprover, got %T", a)
		}
	})

	t.Run("multi", func(t *testing.T) {
		a, ok := buildApprover(&config.ApprovalConfig{Method: "multi", Telegram: tg, Discord: dc})
		if !ok {
			t.Fatal("want ok=true")
		}
		if _, isMulti := a.(*security.MultiApprover); !isMulti {
			t.Errorf("want *MultiApprover, got %T", a)
		}
	})

	t.Run("unknown method", func(t *testing.T) {
		if _, ok := buildApprover(&config.ApprovalConfig{Method: "nope"}); ok {
			t.Error("want ok=false for unknown method")
		}
	})
}
