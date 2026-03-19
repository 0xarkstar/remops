package config

import (
	"os"
	"path/filepath"
)

// DefaultConfigPaths returns the list of paths to search for remops.yaml, in priority order.
func DefaultConfigPaths() []string {
	var paths []string

	// 1. REMOPS_CONFIG env var
	if env := os.Getenv("REMOPS_CONFIG"); env != "" {
		paths = append(paths, env)
	}

	// 2. Current directory
	paths = append(paths, "remops.yaml")

	// 3. XDG_CONFIG_HOME or ~/.config
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			configDir = filepath.Join(home, ".config")
		}
	}
	if configDir != "" {
		paths = append(paths, filepath.Join(configDir, "remops", "remops.yaml"))
	}

	return paths
}

// DefaultDataDir returns the XDG data directory for remops.
func DefaultDataDir() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			dataDir = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Join(dataDir, "remops")
}

// DefaultStateDir returns the XDG state directory for remops.
func DefaultStateDir() string {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			stateDir = filepath.Join(home, ".local", "state")
		}
	}
	return filepath.Join(stateDir, "remops")
}

// AuditLogPath returns the path to the audit log file.
func AuditLogPath() string {
	return filepath.Join(DefaultDataDir(), "audit.jsonl")
}

// RateLimitStatePath returns the path to the rate limiter state file.
func RateLimitStatePath() string {
	return filepath.Join(DefaultStateDir(), "ratelimit.json")
}
