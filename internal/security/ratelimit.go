package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/0xarkstar/remops/internal/config"
)

type writeRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Command   string    `json:"command"`
}

// state maps host name to its write records.
type rateLimitState map[string][]writeRecord

// RateLimiter tracks write operations per host.
type RateLimiter struct {
	mu                sync.Mutex
	maxPerHostPerHour int
	state             rateLimitState
}

// NewRateLimiter loads existing state from the state file, pruning entries older than 1 hour.
func NewRateLimiter(maxPerHostPerHour int) (*RateLimiter, error) {
	path := config.RateLimitStatePath()
	state := make(rateLimitState)

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read rate limit state: %w", err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &state); err != nil {
			// Corrupt state — start fresh.
			state = make(rateLimitState)
		}
	}

	rl := &RateLimiter{
		maxPerHostPerHour: maxPerHostPerHour,
		state:             state,
	}
	rl.prune()
	return rl, nil
}

// Check returns an error if the host has exceeded its write rate limit.
func (r *RateLimiter) Check(host string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneHost(host)
	if len(r.state[host]) >= r.maxPerHostPerHour {
		return fmt.Errorf("rate limit exceeded for host %q: %d writes in the last hour (max %d)",
			host, len(r.state[host]), r.maxPerHostPerHour)
	}
	return nil
}

// Record adds a write record for the host and persists state.
func (r *RateLimiter) Record(host, command string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneHost(host)
	r.state[host] = append(r.state[host], writeRecord{
		Timestamp: time.Now().UTC(),
		Command:   command,
	})
	return r.save()
}

// prune removes all entries older than 1 hour across all hosts.
func (r *RateLimiter) prune() {
	for host := range r.state {
		r.pruneHost(host)
	}
}

// pruneHost removes entries older than 1 hour for a single host.
func (r *RateLimiter) pruneHost(host string) {
	cutoff := time.Now().UTC().Add(-time.Hour)
	records := r.state[host]
	kept := records[:0]
	for _, rec := range records {
		if rec.Timestamp.After(cutoff) {
			kept = append(kept, rec)
		}
	}
	if len(kept) == 0 {
		delete(r.state, host)
	} else {
		r.state[host] = kept
	}
}

func (r *RateLimiter) save() error {
	path := config.RateLimitStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create rate limit state dir: %w", err)
	}
	data, err := json.Marshal(r.state)
	if err != nil {
		return fmt.Errorf("marshal rate limit state: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}
