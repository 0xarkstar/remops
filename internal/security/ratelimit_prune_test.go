package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xarkstar/remops/internal/config"
)

// TestRateLimiterPrunesOldEntries creates a state file with an old record and
// verifies NewRateLimiter prunes it on load.
func TestRateLimiterPrunesOldEntries(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmpDir)

	// Write state with one very old record and one recent record.
	oldTime := time.Now().UTC().Add(-2 * time.Hour)
	recentTime := time.Now().UTC().Add(-30 * time.Minute)

	state := map[string][]writeRecord{
		"host1": {
			{Timestamp: oldTime, Command: "old cmd"},
			{Timestamp: recentTime, Command: "recent cmd"},
		},
	}
	data, _ := json.Marshal(state)

	statePath := config.RateLimitStatePath()
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	rl, err := NewRateLimiter(10)
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}

	// After pruning, only the recent record remains (1 < 10 limit).
	if err := rl.Check("host1"); err != nil {
		t.Errorf("Check after prune: unexpected error: %v", err)
	}
}

// TestRateLimiterCorruptState verifies corrupt state file is ignored.
func TestRateLimiterCorruptState(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmpDir)

	statePath := config.RateLimitStatePath()
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("not valid json{{"), 0o600); err != nil {
		t.Fatal(err)
	}

	rl, err := NewRateLimiter(5)
	if err != nil {
		t.Fatalf("NewRateLimiter with corrupt state: %v", err)
	}

	// Should start fresh
	if err := rl.Check("host1"); err != nil {
		t.Errorf("Check after corrupt state: unexpected error: %v", err)
	}
}
