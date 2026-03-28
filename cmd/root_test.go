package cmd

import (
	"testing"

	"github.com/0xarkstar/remops/internal/config"
)

func TestResolveHosts_NilConfig(t *testing.T) {
	origCfg := cfg
	cfg = nil
	t.Cleanup(func() { cfg = origCfg })

	hosts := resolveHosts()
	if hosts != nil {
		t.Errorf("expected nil, got %v", hosts)
	}
}

func TestResolveHosts_ByFlagHost_Found(t *testing.T) {
	origCfg := cfg
	origHost := flagHost
	cfg = &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {Address: "1.2.3.4"},
			"dev":  {Address: "5.6.7.8"},
		},
	}
	flagHost = "prod"
	t.Cleanup(func() {
		cfg = origCfg
		flagHost = origHost
	})

	hosts := resolveHosts()
	if len(hosts) != 1 || hosts[0] != "prod" {
		t.Errorf("got %v, want [prod]", hosts)
	}
}

func TestResolveHosts_ByFlagHost_NotFound(t *testing.T) {
	origCfg := cfg
	origHost := flagHost
	cfg = &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {Address: "1.2.3.4"},
		},
	}
	flagHost = "missing"
	t.Cleanup(func() {
		cfg = origCfg
		flagHost = origHost
	})

	hosts := resolveHosts()
	if len(hosts) != 0 {
		t.Errorf("expected empty, got %v", hosts)
	}
}

func TestResolveHosts_ByTag(t *testing.T) {
	origCfg := cfg
	origHost := flagHost
	origTag := flagTag
	cfg = &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {Address: "1.2.3.4", Tags: []string{"production"}},
			"dev":  {Address: "5.6.7.8", Tags: []string{"staging"}},
		},
	}
	flagHost = ""
	flagTag = "production"
	t.Cleanup(func() {
		cfg = origCfg
		flagHost = origHost
		flagTag = origTag
	})

	hosts := resolveHosts()
	if len(hosts) != 1 || hosts[0] != "prod" {
		t.Errorf("got %v, want [prod]", hosts)
	}
}

func TestResolveHosts_AllHosts(t *testing.T) {
	origCfg := cfg
	origHost := flagHost
	origTag := flagTag
	cfg = &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {Address: "1.2.3.4"},
			"dev":  {Address: "5.6.7.8"},
		},
	}
	flagHost = ""
	flagTag = ""
	t.Cleanup(func() {
		cfg = origCfg
		flagHost = origHost
		flagTag = origTag
	})

	hosts := resolveHosts()
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d: %v", len(hosts), hosts)
	}
}

func TestGetFormatter_ReturnsNonNil(t *testing.T) {
	tests := []struct {
		name   string
		format string
	}{
		{"json format", "json"},
		{"table format", "table"},
		{"auto format", "auto"},
	}

	origFormat := flagFormat
	t.Cleanup(func() { flagFormat = origFormat })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagFormat = tt.format
			f := getFormatter()
			if f == nil {
				t.Error("getFormatter() returned nil")
			}
			_ = f
		})
	}
}

func TestSetVersionInfo(t *testing.T) {
	origVersion := buildVersion
	origCommit := buildCommit
	origDate := buildDate
	t.Cleanup(func() {
		buildVersion = origVersion
		buildCommit = origCommit
		buildDate = origDate
	})

	SetVersionInfo("1.2.3", "abc123", "2024-01-01")

	if buildVersion != "1.2.3" {
		t.Errorf("buildVersion = %q, want %q", buildVersion, "1.2.3")
	}
	if buildCommit != "abc123" {
		t.Errorf("buildCommit = %q, want %q", buildCommit, "abc123")
	}
	if buildDate != "2024-01-01" {
		t.Errorf("buildDate = %q, want %q", buildDate, "2024-01-01")
	}
	if rootCmd.Version != "1.2.3" {
		t.Errorf("rootCmd.Version = %q, want %q", rootCmd.Version, "1.2.3")
	}
}
