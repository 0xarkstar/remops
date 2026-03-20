package security

import "testing"

func TestRateLimiterAllowsUpToMax(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	rl, err := NewRateLimiter(3)
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := rl.Check("host1"); err != nil {
			t.Fatalf("Check %d: unexpected error: %v", i, err)
		}
		if err := rl.Record("host1", "docker restart app"); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}

	// 4th check should be denied
	if err := rl.Check("host1"); err == nil {
		t.Error("expected rate limit error after 3 writes, got nil")
	}
}

func TestRateLimiterIndependentHosts(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	rl, err := NewRateLimiter(2)
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}

	// Fill up host1
	for i := 0; i < 2; i++ {
		rl.Check("host1")
		rl.Record("host1", "cmd")
	}

	// host2 should be independent
	if err := rl.Check("host2"); err != nil {
		t.Errorf("host2 should not be rate-limited: %v", err)
	}

	// host1 should be limited
	if err := rl.Check("host1"); err == nil {
		t.Error("host1 should be rate-limited")
	}
}

func TestRateLimiterFreshState(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	rl, err := NewRateLimiter(5)
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}

	// Fresh limiter should allow all checks
	for _, host := range []string{"host1", "host2", "host3"} {
		if err := rl.Check(host); err != nil {
			t.Errorf("fresh limiter denied %s: %v", host, err)
		}
	}
}

func TestRateLimiterRecord(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	rl, err := NewRateLimiter(10)
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}

	if err := rl.Record("host1", "docker ps"); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// After recording, check should still pass (1 < 10)
	if err := rl.Check("host1"); err != nil {
		t.Errorf("Check after 1 record: unexpected error: %v", err)
	}
}
