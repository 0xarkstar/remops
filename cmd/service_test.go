package cmd

import (
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
)

func TestResolveService_NilConfig(t *testing.T) {
	origCfg := cfg
	cfg = nil
	t.Cleanup(func() { cfg = origCfg })

	_, err := resolveService("anything")
	if err == nil {
		t.Fatal("expected error when cfg is nil")
	}
	if !strings.Contains(err.Error(), "config not loaded") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveService_UnknownService(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Services: map[string]config.Service{
			"app": {Host: "prod", Container: "app_container"},
		},
	}
	t.Cleanup(func() { cfg = origCfg })

	_, err := resolveService("unknown")
	if err == nil {
		t.Fatal("expected error for unknown service")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveService_KnownService(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Services: map[string]config.Service{
			"app": {Host: "prod", Container: "app_container"},
		},
	}
	t.Cleanup(func() { cfg = origCfg })

	svc, err := resolveService("app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.Container != "app_container" {
		t.Errorf("got container %q, want %q", svc.Container, "app_container")
	}
}

func TestResolveService_NoServicesInConfig(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Services: map[string]config.Service{},
	}
	t.Cleanup(func() { cfg = origCfg })

	_, err := resolveService("app")
	if err == nil {
		t.Fatal("expected error for missing service")
	}
	if !strings.Contains(err.Error(), "No services defined") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCurrentProfileLevel_NilConfig(t *testing.T) {
	origCfg := cfg
	cfg = nil
	t.Cleanup(func() { cfg = origCfg })

	level := currentProfileLevel()
	if level != config.LevelAdmin {
		t.Errorf("got %v, want LevelAdmin", level)
	}
}

func TestCurrentProfileLevel_UnknownProfile(t *testing.T) {
	origCfg := cfg
	origProfile := flagProfile
	cfg = &config.Config{
		Version:  1,
		Hosts:    map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Profiles: map[string]config.Profile{},
	}
	flagProfile = "nonexistent"
	t.Cleanup(func() {
		cfg = origCfg
		flagProfile = origProfile
	})

	level := currentProfileLevel()
	if level != config.LevelAdmin {
		t.Errorf("got %v, want LevelAdmin", level)
	}
}

func TestCurrentProfileLevel_KnownProfile(t *testing.T) {
	origCfg := cfg
	origProfile := flagProfile
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Profiles: map[string]config.Profile{
			"readonly": {Level: "viewer"},
		},
	}
	flagProfile = "readonly"
	t.Cleanup(func() {
		cfg = origCfg
		flagProfile = origProfile
	})

	level := currentProfileLevel()
	if level != config.LevelViewer {
		t.Errorf("got %v, want LevelViewer", level)
	}
}

func TestBuildLogsCmd(t *testing.T) {
	tests := []struct {
		name      string
		container string
		tail      int
		since     string
		wantParts []string
	}{
		{
			name:      "basic",
			container: "myapp",
			tail:      100,
			since:     "",
			wantParts: []string{"docker", "logs", "myapp", "--tail", "100"},
		},
		{
			name:      "with --since",
			container: "myapp",
			tail:      50,
			since:     "1h",
			wantParts: []string{"docker", "logs", "myapp", "--tail", "50", "--since", "1h"},
		},
		{
			name:      "with --tail only",
			container: "svc",
			tail:      200,
			since:     "",
			wantParts: []string{"docker", "logs", "svc", "--tail", "200"},
		},
		{
			name:      "with both tail and since",
			container: "db",
			tail:      10,
			since:     "2024-01-01T00:00:00",
			wantParts: []string{"docker", "logs", "db", "--tail", "10", "--since", "2024-01-01T00:00:00"},
		},
	}

	origTail := flagServiceLogsTail
	origSince := flagServiceLogsSince
	t.Cleanup(func() {
		flagServiceLogsTail = origTail
		flagServiceLogsSince = origSince
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagServiceLogsTail = tt.tail
			flagServiceLogsSince = tt.since

			got := buildLogsCmd(tt.container)
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("buildLogsCmd(%q) = %q, missing part %q", tt.container, got, part)
				}
			}
			// verify no --since when since is empty
			if tt.since == "" && strings.Contains(got, "--since") {
				t.Errorf("buildLogsCmd(%q) = %q, unexpected --since flag", tt.container, got)
			}
		})
	}
}
