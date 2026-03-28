package security

import (
	"strings"
	"testing"
)

func TestTruncateOutput_Short(t *testing.T) {
	input := "hello world"
	got := TruncateOutput(input)
	if got != input {
		t.Errorf("short output should not be truncated")
	}
}

func TestTruncateOutput_ExactLimit(t *testing.T) {
	input := strings.Repeat("x", MaxOutputBytes)
	got := TruncateOutput(input)
	if got != input {
		t.Errorf("exact limit should not be truncated")
	}
}

func TestTruncateOutput_OverLimit(t *testing.T) {
	input := strings.Repeat("x", MaxOutputBytes+100)
	got := TruncateOutput(input)
	if len(got) > MaxOutputBytes+100 {
		t.Errorf("truncated output too long: %d", len(got))
	}
	if !strings.Contains(got, "[output truncated") {
		t.Errorf("missing truncation notice")
	}
}
