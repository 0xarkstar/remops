package config

import (
	"testing"
	"time"
)

func TestHostEffectivePort(t *testing.T) {
	tests := []struct {
		port int
		want int
	}{
		{0, 22},
		{22, 22},
		{2222, 2222},
		{8022, 8022},
	}
	for _, tc := range tests {
		h := Host{Address: "1.2.3.4", Port: tc.port}
		if got := h.EffectivePort(); got != tc.want {
			t.Errorf("port=%d: want %d, got %d", tc.port, tc.want, got)
		}
	}
}

func TestHostEffectiveUser(t *testing.T) {
	tests := []struct {
		user string
		want string
	}{
		{"", "root"},
		{"ubuntu", "ubuntu"},
		{"deploy", "deploy"},
	}
	for _, tc := range tests {
		h := Host{Address: "1.2.3.4", User: tc.user}
		if got := h.EffectiveUser(); got != tc.want {
			t.Errorf("user=%q: want %q, got %q", tc.user, tc.want, got)
		}
	}
}

func TestHostEffectiveTimeout(t *testing.T) {
	tests := []struct {
		timeout string
		want    time.Duration
	}{
		{"", 10 * time.Second},
		{"30s", 30 * time.Second},
		{"2m", 2 * time.Minute},
		{"not-valid", 10 * time.Second},
	}
	for _, tc := range tests {
		h := Host{Address: "1.2.3.4", Timeout: tc.timeout}
		if got := h.EffectiveTimeout(); got != tc.want {
			t.Errorf("timeout=%q: want %v, got %v", tc.timeout, tc.want, got)
		}
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  PermissionLevel
	}{
		{"viewer", LevelViewer},
		{"operator", LevelOperator},
		{"admin", LevelAdmin},
		{"unknown", LevelViewer},
		{"", LevelViewer},
		{"ADMIN", LevelViewer}, // case-sensitive
	}
	for _, tc := range tests {
		if got := ParseLevel(tc.input); got != tc.want {
			t.Errorf("ParseLevel(%q): want %v, got %v", tc.input, tc.want, got)
		}
	}
}

func TestPermissionLevelString(t *testing.T) {
	tests := []struct {
		level PermissionLevel
		want  string
	}{
		{LevelViewer, "viewer"},
		{LevelOperator, "operator"},
		{LevelAdmin, "admin"},
		{PermissionLevel(99), "viewer"}, // unknown → viewer
	}
	for _, tc := range tests {
		if got := tc.level.String(); got != tc.want {
			t.Errorf("PermissionLevel(%d).String(): want %q, got %q", tc.level, tc.want, got)
		}
	}
}

func TestApprovalConfigEffectiveTimeout(t *testing.T) {
	tests := []struct {
		timeout string
		want    time.Duration
	}{
		{"", 5 * time.Minute},
		{"10m", 10 * time.Minute},
		{"30s", 30 * time.Second},
		{"bad", 5 * time.Minute},
	}
	for _, tc := range tests {
		a := ApprovalConfig{Timeout: tc.timeout}
		if got := a.EffectiveTimeout(); got != tc.want {
			t.Errorf("timeout=%q: want %v, got %v", tc.timeout, tc.want, got)
		}
	}
}

func TestRateLimitConfigEffectiveMaxWrites(t *testing.T) {
	tests := []struct {
		max  int
		want int
	}{
		{0, 5},
		{-1, 5},
		{10, 10},
		{1, 1},
	}
	for _, tc := range tests {
		r := RateLimitConfig{MaxWritesPerHostPerHour: tc.max}
		if got := r.EffectiveMaxWrites(); got != tc.want {
			t.Errorf("max=%d: want %d, got %d", tc.max, tc.want, got)
		}
	}
}
