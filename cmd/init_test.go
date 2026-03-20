package cmd

import (
	"bufio"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	"gopkg.in/yaml.v3"
)

func TestBuildDefaultConfig(t *testing.T) {
	hosts := map[string]config.Host{
		"prod": {
			Address: "192.168.1.1",
			User:    "deploy",
		},
	}

	cfg := buildDefaultConfig(hosts)

	if cfg.Version != 1 {
		t.Errorf("expected version 1, got %d", cfg.Version)
	}

	if len(cfg.Hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(cfg.Hosts))
	}

	if _, ok := cfg.Hosts["prod"]; !ok {
		t.Error("expected host 'prod' to exist")
	}

	expectedProfiles := []string{"viewer", "operator", "admin"}
	if len(cfg.Profiles) != len(expectedProfiles) {
		t.Errorf("expected %d profiles, got %d", len(expectedProfiles), len(cfg.Profiles))
	}
	for _, name := range expectedProfiles {
		p, ok := cfg.Profiles[name]
		if !ok {
			t.Errorf("expected profile %q to exist", name)
			continue
		}
		if p.Level != name {
			t.Errorf("profile %q: expected level %q, got %q", name, name, p.Level)
		}
	}
}

func TestBuildDefaultConfigProducesValidYAML(t *testing.T) {
	hosts := map[string]config.Host{
		"dev": {
			Address: "10.0.0.1",
			User:    "ubuntu",
			Port:    2222,
		},
	}

	cfg := buildDefaultConfig(hosts)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	var parsed config.Config
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	if parsed.Version != 1 {
		t.Errorf("roundtrip: expected version 1, got %d", parsed.Version)
	}

	if len(parsed.Profiles) != 3 {
		t.Errorf("roundtrip: expected 3 profiles, got %d", len(parsed.Profiles))
	}

	h, ok := parsed.Hosts["dev"]
	if !ok {
		t.Fatal("roundtrip: expected host 'dev'")
	}
	if h.Address != "10.0.0.1" {
		t.Errorf("roundtrip: expected address '10.0.0.1', got %q", h.Address)
	}
	if h.Port != 2222 {
		t.Errorf("roundtrip: expected port 2222, got %d", h.Port)
	}
}

func TestPrompt_UserInput(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("myvalue\n"))
	got := prompt(scanner, "Enter value", "default")
	if got != "myvalue" {
		t.Errorf("prompt() = %q, want %q", got, "myvalue")
	}
}

func TestPrompt_EmptyInput_UsesDefault(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("\n"))
	got := prompt(scanner, "Enter value", "default")
	if got != "default" {
		t.Errorf("prompt() = %q, want %q", got, "default")
	}
}

func TestPrompt_EmptyInput_NoDefault(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("\n"))
	got := prompt(scanner, "Enter value", "")
	if got != "" {
		t.Errorf("prompt() = %q, want empty string", got)
	}
}

func TestPrompt_WhitespaceStripped(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("  trimmed  \n"))
	got := prompt(scanner, "Enter value", "default")
	if got != "trimmed" {
		t.Errorf("prompt() = %q, want %q", got, "trimmed")
	}
}
