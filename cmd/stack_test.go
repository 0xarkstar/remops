package cmd

import (
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
)

func TestResolveStack_NilConfig(t *testing.T) {
	origCfg := cfg
	cfg = nil
	t.Cleanup(func() { cfg = origCfg })

	_, err := resolveStack("anything")
	if err == nil {
		t.Fatal("expected error when cfg is nil")
	}
	if !strings.Contains(err.Error(), "config not loaded") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveStack_Unknown(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Stacks: map[string]config.Stack{
			"monitoring": {Host: "prod", Path: "/opt/monitoring"},
		},
	}
	t.Cleanup(func() { cfg = origCfg })

	_, err := resolveStack("unknown")
	if err == nil {
		t.Fatal("expected error for unknown stack")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "monitoring") {
		t.Errorf("expected available stacks in error, got: %v", err)
	}
}

func TestResolveStack_Known(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Stacks: map[string]config.Stack{
			"monitoring": {Host: "prod", Path: "/opt/monitoring"},
		},
	}
	t.Cleanup(func() { cfg = origCfg })

	stack, err := resolveStack("monitoring")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stack.Path != "/opt/monitoring" {
		t.Errorf("got path %q, want %q", stack.Path, "/opt/monitoring")
	}
	if stack.Host != "prod" {
		t.Errorf("got host %q, want %q", stack.Host, "prod")
	}
}

func TestResolveStack_NoStacksInConfig(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
		Stacks:  map[string]config.Stack{},
	}
	t.Cleanup(func() { cfg = origCfg })

	_, err := resolveStack("anything")
	if err == nil {
		t.Fatal("expected error when no stacks defined")
	}
	if !strings.Contains(err.Error(), "No stacks defined") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStackCmdStructure(t *testing.T) {
	subcommands := map[string]bool{}
	for _, sub := range stackCmd.Commands() {
		subcommands[sub.Name()] = true
	}

	required := []string{"ps", "logs", "up", "pull", "restart", "down"}
	for _, name := range required {
		if !subcommands[name] {
			t.Errorf("stack command missing subcommand %q", name)
		}
	}
}
