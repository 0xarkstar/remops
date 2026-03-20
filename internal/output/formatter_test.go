package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
)

func init() {
	// Disable color output in tests for predictable assertions.
	color.NoColor = true
}

func TestJSONFormatterFormat(t *testing.T) {
	r := NewResponse()
	r.AddResult(map[string]any{"host": "web1", "status": "running"})
	r.Finalize(time.Now().Add(-50 * time.Millisecond))

	var buf bytes.Buffer
	f := &JSONFormatter{}
	if err := f.Format(&buf, r); err != nil {
		t.Fatalf("Format: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}
	if _, ok := out["results"]; !ok {
		t.Error("expected 'results' in JSON output")
	}
}

func TestTableFormatterFormat(t *testing.T) {
	r := NewResponse()
	r.AddResult(map[string]any{"host": "web1", "state": "running"})
	r.AddFailure("db1", "CONN", "timeout", "check firewall")
	r.Finalize(time.Now().Add(-10 * time.Millisecond))

	var buf bytes.Buffer
	f := &TableFormatter{}
	if err := f.Format(&buf, r); err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "web1") {
		t.Errorf("expected 'web1' in table output, got: %s", out)
	}
	if !strings.Contains(out, "Failures") {
		t.Errorf("expected 'Failures' in table output, got: %s", out)
	}
	if !strings.Contains(out, "db1") {
		t.Errorf("expected 'db1' in table output, got: %s", out)
	}
	if !strings.Contains(out, "check firewall") {
		t.Errorf("expected suggestion in table output, got: %s", out)
	}
}

func TestTableFormatterSliceResult(t *testing.T) {
	r := NewResponse()
	r.AddResult([]any{
		map[string]any{"name": "app1"},
		"plain string",
	})

	var buf bytes.Buffer
	f := &TableFormatter{}
	if err := f.Format(&buf, r); err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "app1") {
		t.Errorf("expected 'app1' in output, got: %s", out)
	}
	if !strings.Contains(out, "plain string") {
		t.Errorf("expected 'plain string' in output, got: %s", out)
	}
}

func TestTableFormatterScalarResult(t *testing.T) {
	r := NewResponse()
	r.AddResult("just a string")

	var buf bytes.Buffer
	f := &TableFormatter{}
	if err := f.Format(&buf, r); err != nil {
		t.Fatalf("Format: %v", err)
	}
	if !strings.Contains(buf.String(), "just a string") {
		t.Errorf("expected scalar in output, got: %s", buf.String())
	}
}

func TestTableFormatterNoSummaryWhenEmpty(t *testing.T) {
	r := NewResponse()
	r.Finalize(time.Now())

	var buf bytes.Buffer
	f := &TableFormatter{}
	if err := f.Format(&buf, r); err != nil {
		t.Fatalf("Format: %v", err)
	}
	if strings.Contains(buf.String(), "succeeded") {
		t.Errorf("expected no summary for empty response, got: %s", buf.String())
	}
}

func TestNewFormatterJSON(t *testing.T) {
	f := NewFormatter(FormatJSON)
	if _, ok := f.(*JSONFormatter); !ok {
		t.Errorf("NewFormatter(json): want *JSONFormatter, got %T", f)
	}
}

func TestNewFormatterTable(t *testing.T) {
	f := NewFormatter(FormatTable)
	if _, ok := f.(*TableFormatter); !ok {
		t.Errorf("NewFormatter(table): want *TableFormatter, got %T", f)
	}
}

func TestNewFormatterAuto(t *testing.T) {
	// In tests stdout is not a TTY, so auto should resolve to JSON.
	f := NewFormatter(FormatAuto)
	if f == nil {
		t.Error("NewFormatter(auto): got nil")
	}
}

func TestResolveFormatNonAuto(t *testing.T) {
	if got := resolveFormat(FormatJSON); got != FormatJSON {
		t.Errorf("resolveFormat(json): want json, got %s", got)
	}
	if got := resolveFormat(FormatTable); got != FormatTable {
		t.Errorf("resolveFormat(table): want table, got %s", got)
	}
}
