package config

import "testing"

func TestEffectiveTelegram(t *testing.T) {
	t.Run("nested block preferred", func(t *testing.T) {
		a := &ApprovalConfig{
			BotToken: "flat", ChatID: "flatchat",
			Telegram: &TelegramApprovalConfig{BotToken: "nested", ChatID: "nestedchat"},
		}
		token, chat, ok := a.EffectiveTelegram()
		if !ok || token != "nested" || chat != "nestedchat" {
			t.Errorf("want nested creds, got token=%q chat=%q ok=%v", token, chat, ok)
		}
	})
	t.Run("flat fallback", func(t *testing.T) {
		a := &ApprovalConfig{BotToken: "flat", ChatID: "flatchat"}
		token, chat, ok := a.EffectiveTelegram()
		if !ok || token != "flat" || chat != "flatchat" {
			t.Errorf("want flat creds, got token=%q chat=%q ok=%v", token, chat, ok)
		}
	})
	t.Run("none", func(t *testing.T) {
		a := &ApprovalConfig{}
		if _, _, ok := a.EffectiveTelegram(); ok {
			t.Error("want ok=false when no telegram creds")
		}
	})
}

func TestEffectiveDiscord(t *testing.T) {
	a := &ApprovalConfig{Discord: &DiscordApprovalConfig{BotToken: "dt", ChannelID: "dc"}}
	token, chan_, ok := a.EffectiveDiscord()
	if !ok || token != "dt" || chan_ != "dc" {
		t.Errorf("want discord creds, got token=%q chan=%q ok=%v", token, chan_, ok)
	}
	if _, _, ok := (&ApprovalConfig{}).EffectiveDiscord(); ok {
		t.Error("want ok=false when no discord creds")
	}
}

func TestValidateApproval(t *testing.T) {
	tg := &TelegramApprovalConfig{BotToken: "t", ChatID: "c"}
	dc := &DiscordApprovalConfig{BotToken: "d", ChannelID: "ch"}

	cases := []struct {
		name    string
		a       *ApprovalConfig
		wantErr bool
	}{
		{"telegram flat ok", &ApprovalConfig{Method: "telegram", BotToken: "t", ChatID: "c"}, false},
		{"telegram nested ok", &ApprovalConfig{Method: "telegram", Telegram: tg}, false},
		{"telegram missing chat", &ApprovalConfig{Method: "telegram", BotToken: "t"}, true},
		{"telegram missing token", &ApprovalConfig{Method: "telegram", ChatID: "c"}, true},
		{"discord ok", &ApprovalConfig{Method: "discord", Discord: dc}, false},
		{"discord missing", &ApprovalConfig{Method: "discord"}, true},
		{"discord missing channel", &ApprovalConfig{Method: "discord", Discord: &DiscordApprovalConfig{BotToken: "d"}}, true},
		{"multi ok", &ApprovalConfig{Method: "multi", Telegram: tg, Discord: dc}, false},
		{"multi missing discord", &ApprovalConfig{Method: "multi", Telegram: tg}, true},
		{"multi missing telegram", &ApprovalConfig{Method: "multi", Discord: dc}, true},
		{"empty method", &ApprovalConfig{Method: ""}, true},
		{"unknown method", &ApprovalConfig{Method: "carrier-pigeon", Telegram: tg}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateApproval(c.a)
			if c.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestValidateApprovalThroughConfig confirms validateApproval is reached by the
// top-level validate() with a minimal otherwise-valid config.
func TestValidateApprovalThroughConfig(t *testing.T) {
	cfg := &Config{
		Version: 1,
		Hosts:   map[string]Host{"h": {Address: "1.2.3.4"}},
		Approval: &ApprovalConfig{
			Method:  "multi",
			Discord: &DiscordApprovalConfig{BotToken: "d", ChannelID: "ch"},
			// telegram missing → must fail
		},
	}
	if err := validate(cfg); err == nil {
		t.Fatal("expected validate() to reject multi without telegram")
	}
}
