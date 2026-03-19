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

// AuditEntry represents a single audit log line.
type AuditEntry struct {
	Timestamp string `json:"timestamp"`
	Command   string `json:"command"`
	Host      string `json:"host,omitempty"`
	Service   string `json:"service,omitempty"`
	Profile   string `json:"profile"`
	Result    string `json:"result"` // "success", "denied", "error", "approved", "denied_approval"
	Duration  int64  `json:"duration_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// AuditLogger writes JSONL audit entries.
type AuditLogger struct {
	mu   sync.Mutex
	file *os.File
}

// NewAuditLogger creates an AuditLogger, creating parent directories as needed.
func NewAuditLogger() (*AuditLogger, error) {
	path := config.AuditLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create audit log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	return &AuditLogger{file: f}, nil
}

// Log appends a JSONL entry to the audit log.
func (a *AuditLogger) Log(entry AuditEntry) error {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err = fmt.Fprintf(a.file, "%s\n", line)
	return err
}

// Close flushes and closes the audit log file.
func (a *AuditLogger) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.file.Close()
}
