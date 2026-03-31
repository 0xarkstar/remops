package cmd

import (
	"bufio"
	"os"
	"path/filepath"
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

func TestDiscoverSSHConfigHosts(t *testing.T) {
	// Create a temp home directory with an .ssh/config file.
	tmpHome := t.TempDir()
	sshDir := filepath.Join(tmpHome, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("failed to create .ssh dir: %v", err)
	}

	sshConfigContent := `Host prod
  HostName 172.30.1.99
  User arkstar
  IdentityFile ~/.ssh/id_ed25519

Host staging
  HostName 10.0.0.5
  User deploy
  Port 2222

Host *
  ServerAliveInterval 60
`
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(sshConfigContent), 0o600); err != nil {
		t.Fatalf("failed to write ssh config: %v", err)
	}

	// Override HOME so discoverSSHConfigHosts reads our temp config.
	t.Setenv("HOME", tmpHome)

	hosts := discoverSSHConfigHosts()

	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d: %+v", len(hosts), hosts)
	}

	byName := make(map[string]sshHostEntry)
	for _, h := range hosts {
		byName[h.Name] = h
	}

	prod, ok := byName["prod"]
	if !ok {
		t.Fatal("expected host 'prod'")
	}
	if prod.Address != "172.30.1.99" {
		t.Errorf("prod address: want 172.30.1.99, got %s", prod.Address)
	}
	if prod.User != "arkstar" {
		t.Errorf("prod user: want arkstar, got %s", prod.User)
	}
	if prod.Port != 22 {
		t.Errorf("prod port: want 22, got %d", prod.Port)
	}
	if prod.Key == "" {
		t.Error("prod key: expected non-empty after ~ expansion")
	}

	staging, ok := byName["staging"]
	if !ok {
		t.Fatal("expected host 'staging'")
	}
	if staging.Address != "10.0.0.5" {
		t.Errorf("staging address: want 10.0.0.5, got %s", staging.Address)
	}
	if staging.User != "deploy" {
		t.Errorf("staging user: want deploy, got %s", staging.User)
	}
	if staging.Port != 2222 {
		t.Errorf("staging port: want 2222, got %d", staging.Port)
	}
}

func TestDiscoverSSHConfigHosts_NoConfigFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	hosts := discoverSSHConfigHosts()
	if hosts != nil {
		t.Errorf("expected nil when no ssh config, got %v", hosts)
	}
}

func TestDiscoverSSHConfigHosts_SkipsHostsWithoutHostName(t *testing.T) {
	tmpHome := t.TempDir()
	sshDir := filepath.Join(tmpHome, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("failed to create .ssh dir: %v", err)
	}

	// alias-only entry (no HostName) should be skipped.
	sshConfigContent := `Host alias-only
  User someone

Host real
  HostName 192.168.1.1
  User ubuntu
`
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(sshConfigContent), 0o600); err != nil {
		t.Fatalf("failed to write ssh config: %v", err)
	}

	t.Setenv("HOME", tmpHome)

	hosts := discoverSSHConfigHosts()
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d: %+v", len(hosts), hosts)
	}
	if hosts[0].Name != "real" {
		t.Errorf("expected host 'real', got %s", hosts[0].Name)
	}
}
