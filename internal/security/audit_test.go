package security

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
)

func TestAuditLogger(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	logger, err := NewAuditLogger()
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}

	entry := AuditEntry{
		Command: "docker ps",
		Host:    "web1",
		Profile: "default",
		Result:  "success",
	}
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read back and verify
	logPath := config.AuditLogPath()
	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open audit log %s: %v", logPath, err)
	}
	defer f.Close()

	var entries []AuditEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e AuditEntry
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("unmarshal line: %v", err)
		}
		entries = append(entries, e)
	}

	if len(entries) != 1 {
		t.Fatalf("want 1 log entry, got %d", len(entries))
	}
	if entries[0].Command != "docker ps" {
		t.Errorf("command: want 'docker ps', got %q", entries[0].Command)
	}
	if entries[0].Host != "web1" {
		t.Errorf("host: want 'web1', got %q", entries[0].Host)
	}
	if entries[0].Profile != "default" {
		t.Errorf("profile: want 'default', got %q", entries[0].Profile)
	}
	if entries[0].Result != "success" {
		t.Errorf("result: want 'success', got %q", entries[0].Result)
	}
	if entries[0].Timestamp == "" {
		t.Error("expected timestamp to be auto-set")
	}
}

func TestAuditLoggerMultipleEntries(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	logger, err := NewAuditLogger()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := logger.Log(AuditEntry{
			Command: "docker restart app",
			Profile: "ops",
			Result:  "success",
		}); err != nil {
			t.Fatalf("Log %d: %v", i, err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(config.AuditLogPath())
	if err != nil {
		t.Fatal(err)
	}

	count := bytes.Count(data, []byte("\n"))
	if count != 3 {
		t.Errorf("want 3 newlines (3 JSONL entries), got %d", count)
	}
}

func TestAuditLoggerTimestampAutoSet(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	logger, err := NewAuditLogger()
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	// Log entry without explicit timestamp — it should be auto-set
	if err := logger.Log(AuditEntry{
		Command: "cmd",
		Profile: "default",
		Result:  "success",
	}); err != nil {
		t.Fatal(err)
	}
	logger.Close()

	data, err := os.ReadFile(config.AuditLogPath())
	if err != nil {
		t.Fatal(err)
	}

	var e AuditEntry
	if err := json.Unmarshal(bytes.TrimRight(data, "\n"), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Timestamp == "" {
		t.Error("expected timestamp to be set automatically")
	}
}
