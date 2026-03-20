package cmd

import (
	"testing"
)

func TestParsePorts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"public port", "0.0.0.0:80->80/tcp", 1},
		{"private port", "127.0.0.1:80->80/tcp", 0},
		{"mixed", "0.0.0.0:443->443/tcp\n127.0.0.1:8080->8080/tcp", 1},
		{"no ports", "", 0},
		{"multiple public", "0.0.0.0:80->80/tcp\n0.0.0.0:443->443/tcp", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePorts(tt.input)
			if len(got) != tt.want {
				t.Errorf("parsePorts(%q) = %d entries, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}

func TestParseLatestImages(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"one latest", "nginx:latest", 1},
		{"multiple", "nginx:latest\nredis:latest", 2},
		{"empty", "", 0},
		{"with whitespace", "  nginx:latest  \n  redis:latest  ", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLatestImages(tt.input)
			if len(got) != tt.want {
				t.Errorf("parseLatestImages(%q) = %d entries, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}

func TestIsStaleStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"Up 2 months", true},
		{"Up 1 months", true},
		{"Up 3 days", false},
		{"Up 2 hours", false},
		{"Exited (0) 2 months ago", false}, // not running
		{"Up 45 minutes", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := isStaleStatus(tt.status); got != tt.want {
				t.Errorf("isStaleStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestParseStaleContainers(t *testing.T) {
	input := "my-app Up 2 months\nredis Up 3 days\nnginx Up 1 months"
	got := parseStaleContainers(input)
	if len(got) != 2 {
		t.Errorf("parseStaleContainers: got %d stale containers, want 2; result: %v", len(got), got)
	}
	if got[0] != "my-app" {
		t.Errorf("parseStaleContainers: first stale = %q, want %q", got[0], "my-app")
	}
	if got[1] != "nginx" {
		t.Errorf("parseStaleContainers: second stale = %q, want %q", got[1], "nginx")
	}
}

func TestFindingsToMaps(t *testing.T) {
	findings := []securityFinding{
		{Severity: "WARN", Check: "public_ports", Message: "1 container with public ports"},
		{Severity: "INFO", Check: "latest_tags", Message: "no latest tags"},
	}
	maps := findingsToMaps(findings)
	if len(maps) != 2 {
		t.Fatalf("findingsToMaps: got %d maps, want 2", len(maps))
	}
	if maps[0]["severity"] != "WARN" {
		t.Errorf("maps[0][severity] = %v, want WARN", maps[0]["severity"])
	}
	if maps[1]["check"] != "latest_tags" {
		t.Errorf("maps[1][check] = %v, want latest_tags", maps[1]["check"])
	}
}
