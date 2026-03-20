package transport

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const poolTTL = 5 * time.Minute

type poolEntry struct {
	client   *ssh.Client
	hop      *ssh.Client // non-nil for proxied (ProxyJump) connections
	lastUsed time.Time
}

// Pool manages reusable SSH connections with a TTL.
type Pool struct {
	mu      sync.Mutex
	entries map[string]*poolEntry
}

// NewPool creates an empty connection pool.
func NewPool() *Pool {
	return &Pool{
		entries: make(map[string]*poolEntry),
	}
}

// Get returns an existing live connection or creates one via dial.
// The dial func returns (client, hop, error); hop is non-nil for ProxyJump connections.
// On each call, stale entries (older than poolTTL) are evicted.
func (p *Pool) Get(key string, dial func() (*ssh.Client, *ssh.Client, error)) (*ssh.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	// Lazy eviction of stale entries.
	for k, e := range p.entries {
		if now.Sub(e.lastUsed) > poolTTL {
			e.client.Close() //nolint:errcheck
			if e.hop != nil {
				e.hop.Close() //nolint:errcheck
			}
			delete(p.entries, k)
		}
	}

	if e, ok := p.entries[key]; ok {
		// Verify the connection is still alive with a keepalive.
		_, _, err := e.client.SendRequest("keepalive@openssh.com", true, nil)
		if err == nil {
			e.lastUsed = now
			return e.client, nil
		}
		// Connection is dead — remove and reconnect.
		e.client.Close() //nolint:errcheck
		if e.hop != nil {
			e.hop.Close() //nolint:errcheck
		}
		delete(p.entries, key)
	}

	client, hop, err := dial()
	if err != nil {
		return nil, fmt.Errorf("pool dial %s: %w", key, err)
	}
	p.entries[key] = &poolEntry{client: client, hop: hop, lastUsed: now}
	return client, nil
}

// Close closes all pooled connections.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for k, e := range p.entries {
		if err := e.client.Close(); err != nil {
			lastErr = err
		}
		if e.hop != nil {
			if err := e.hop.Close(); err != nil {
				lastErr = err
			}
		}
		delete(p.entries, k)
	}
	return lastErr
}
