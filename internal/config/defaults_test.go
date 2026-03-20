package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigPathsWithEnv(t *testing.T) {
	t.Setenv("REMOPS_CONFIG", "/custom/remops.yaml")
	t.Setenv("XDG_CONFIG_HOME", "")

	paths := DefaultConfigPaths()
	if len(paths) == 0 {
		t.Fatal("expected at least one path")
	}
	if paths[0] != "/custom/remops.yaml" {
		t.Errorf("first path: want /custom/remops.yaml, got %s", paths[0])
	}
}

func TestDefaultConfigPathsWithoutEnv(t *testing.T) {
	t.Setenv("REMOPS_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")

	paths := DefaultConfigPaths()
	found := false
	for _, p := range paths {
		if strings.Contains(p, "remops.yaml") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one path containing remops.yaml, got %v", paths)
	}
}

func TestDefaultDataDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	dir := DefaultDataDir()
	if dir != filepath.Join("/custom/data", "remops") {
		t.Errorf("want /custom/data/remops, got %s", dir)
	}
}

func TestDefaultStateDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	dir := DefaultStateDir()
	if dir != filepath.Join("/custom/state", "remops") {
		t.Errorf("want /custom/state/remops, got %s", dir)
	}
}

func TestAuditLogPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	p := AuditLogPath()
	if !strings.HasSuffix(p, "audit.jsonl") {
		t.Errorf("AuditLogPath: want suffix audit.jsonl, got %s", p)
	}
	if !strings.Contains(p, "remops") {
		t.Errorf("AuditLogPath: expected remops in path, got %s", p)
	}
}

func TestRateLimitStatePath(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	p := RateLimitStatePath()
	if !strings.HasSuffix(p, "ratelimit.json") {
		t.Errorf("RateLimitStatePath: want suffix ratelimit.json, got %s", p)
	}
	if !strings.Contains(p, "remops") {
		t.Errorf("RateLimitStatePath: expected remops in path, got %s", p)
	}
}
