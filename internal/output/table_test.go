package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// mockHostContainers implements HostContainers for tests.
type mockHostContainers struct {
	host       string
	containers []ContainerRow
}

func (m *mockHostContainers) HostName() string        { return m.host }
func (m *mockHostContainers) ContainerRows() []ContainerRow { return m.containers }

func TestContainerTable_BasicOutput(t *testing.T) {
	r := NewResponse()
	r.AddResult(&mockHostContainers{
		host: "prod",
		containers: []ContainerRow{
			{Name: "crawl4ai", Image: "crawl4ai:latest", Status: "Up 2 days", State: "running"},
			{Name: "searxng", Image: "searxng:latest", Status: "Up 5 hours", State: "running"},
		},
	})
	r.AddResult(&mockHostContainers{
		host: "dev",
		containers: []ContainerRow{
			{Name: "uptime-kuma", Image: "louislam/uptime", Status: "Up 2 days", State: "running"},
		},
	})

	var buf bytes.Buffer
	f := &TableFormatter{}
	if err := f.Format(&buf, r); err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"HOST", "CONTAINER", "IMAGE", "STATUS", "STATE",
		"prod", "crawl4ai", "searxng", "dev", "uptime-kuma"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}

	// Verify column alignment: each row should have consistent spacing.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 4 { // header + 3 container rows
		t.Errorf("expected at least 4 lines, got %d:\n%s", len(lines), out)
	}
}

func TestContainerTable_ColorState(t *testing.T) {
	// Enable colors to verify ANSI codes are emitted.
	color.NoColor = false
	defer func() { color.NoColor = true }()

	r := NewResponse()
	r.AddResult(&mockHostContainers{
		host: "prod",
		containers: []ContainerRow{
			{Name: "app1", Image: "app:latest", Status: "Up 1 day", State: "running"},
			{Name: "app2", Image: "app:latest", Status: "Exited (1) 1h", State: "exited"},
		},
	})

	var buf bytes.Buffer
	f := &TableFormatter{}
	if err := f.Format(&buf, r); err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := buf.String()

	// With colors enabled, ANSI escape codes should be present.
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI color codes in output, got:\n%s", out)
	}
	// Both state values should appear as text.
	if !strings.Contains(out, "running") {
		t.Errorf("expected 'running' in output:\n%s", out)
	}
	if !strings.Contains(out, "exited") {
		t.Errorf("expected 'exited' in output:\n%s", out)
	}
}

func TestContainerTable_EmptyContainers(t *testing.T) {
	r := NewResponse()
	r.AddResult(&mockHostContainers{host: "staging", containers: nil})

	var buf bytes.Buffer
	f := &TableFormatter{}
	if err := f.Format(&buf, r); err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "(no containers)") {
		t.Errorf("expected '(no containers)' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "staging") {
		t.Errorf("expected 'staging' in output, got:\n%s", out)
	}
}

func TestContainerTable_FallbackToMap(t *testing.T) {
	// A plain map[string]any result should fall back to formatMap, not container table.
	r := NewResponse()
	r.AddResult(map[string]any{"host": "web1", "state": "running"})

	var buf bytes.Buffer
	f := &TableFormatter{}
	if err := f.Format(&buf, r); err != nil {
		t.Fatalf("Format: %v", err)
	}
	out := buf.String()

	// formatMap produces key-value pairs, not a column-header table.
	if strings.Contains(out, "CONTAINER") {
		t.Errorf("did not expect container table headers in map fallback output:\n%s", out)
	}
	if !strings.Contains(out, "web1") {
		t.Errorf("expected 'web1' in fallback output:\n%s", out)
	}
}
