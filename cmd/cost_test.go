package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/spf13/cobra"
)

func TestMetaString(t *testing.T) {
	meta := map[string]any{
		"provider": "hetzner",
		"plan":     "CPX31",
	}
	if got := metaString(meta, "provider"); got != "hetzner" {
		t.Errorf("metaString provider = %q, want %q", got, "hetzner")
	}
	if got := metaString(meta, "missing"); got != "" {
		t.Errorf("metaString missing = %q, want empty", got)
	}
}

func TestMetaFloat(t *testing.T) {
	tests := []struct {
		name  string
		meta  map[string]any
		key   string
		want  float64
	}{
		{"float64", map[string]any{"cost_monthly": float64(14.49)}, "cost_monthly", 14.49},
		{"int", map[string]any{"cost_monthly": int(5)}, "cost_monthly", 5.0},
		{"float32", map[string]any{"cost_monthly": float32(4.99)}, "cost_monthly", float64(float32(4.99))},
		{"missing", map[string]any{}, "cost_monthly", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := metaFloat(tt.meta, tt.key); got != tt.want {
				t.Errorf("metaFloat %s = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestFormatCostStr(t *testing.T) {
	tests := []struct {
		entry hostCost
		want  string
	}{
		{hostCost{Cost: 14.49, Currency: "€"}, "€14.49"},
		{hostCost{Cost: 0}, "-"},
		{hostCost{Cost: 4.99, Currency: "$"}, "$4.99"},
	}
	for _, tt := range tests {
		if got := formatCostStr(tt.entry); got != tt.want {
			t.Errorf("formatCostStr(%+v) = %q, want %q", tt.entry, got, tt.want)
		}
	}
}

func TestPrintCostTable(t *testing.T) {
	entries := []hostCost{
		{Host: "hetzner-fra", Provider: "hetzner", Plan: "CPX31", Cost: 14.49, Currency: "€"},
		{Host: "contabo", Provider: "contabo", Plan: "VPS S", Cost: 4.99, Currency: "€"},
	}
	var buf bytes.Buffer
	printCostTable(&buf, entries)
	out := buf.String()

	for _, want := range []string{"hetzner-fra", "contabo", "TOTAL", "€14.49", "€4.99", "€19.48"} {
		if !strings.Contains(out, want) {
			t.Errorf("printCostTable output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestPrintCostJSON(t *testing.T) {
	entries := []hostCost{
		{Host: "host1", Cost: 10.0, Currency: "$"},
		{Host: "host2", Cost: 5.0, Currency: "$"},
	}
	var buf bytes.Buffer
	if err := printCostJSON(&buf, entries); err != nil {
		t.Fatalf("printCostJSON error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`"host1"`, `"host2"`, `"total"`, `15`} {
		if !strings.Contains(out, want) {
			t.Errorf("printCostJSON output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestRunCost_JSON(t *testing.T) {
	origCfg := cfg
	origFormat := flagFormat
	origHost := flagHost
	origTag := flagTag
	origProfile := flagProfile
	cfg = &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {
				Address: "1.2.3.4",
				Meta: map[string]any{
					"provider":     "hetzner",
					"cost_monthly": float64(9.99),
					"currency":     "€",
				},
			},
		},
		Profiles: map[string]config.Profile{
			"admin": {Level: "admin"},
		},
	}
	flagFormat = "json"
	flagHost = ""
	flagTag = ""
	flagProfile = "admin"
	t.Cleanup(func() {
		cfg = origCfg
		flagFormat = origFormat
		flagHost = origHost
		flagTag = origTag
		flagProfile = origProfile
	})

	// Capture stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	cmd := &cobra.Command{}
	runErr := costCmd.RunE(cmd, nil)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if runErr != nil {
		t.Fatalf("runCost error: %v", runErr)
	}
	out := buf.String()
	if !strings.Contains(out, "prod") {
		t.Errorf("output missing host 'prod': %s", out)
	}
	if !strings.Contains(out, "total") {
		t.Errorf("output missing 'total': %s", out)
	}
}

func TestRunCost_Table(t *testing.T) {
	origCfg := cfg
	origFormat := flagFormat
	origHost := flagHost
	origTag := flagTag
	origProfile := flagProfile
	cfg = &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {Address: "1.2.3.4"},
		},
		Profiles: map[string]config.Profile{
			"admin": {Level: "admin"},
		},
	}
	flagFormat = "table"
	flagHost = ""
	flagTag = ""
	flagProfile = "admin"
	t.Cleanup(func() {
		cfg = origCfg
		flagFormat = origFormat
		flagHost = origHost
		flagTag = origTag
		flagProfile = origProfile
	})

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	cmd := &cobra.Command{}
	runErr := costCmd.RunE(cmd, nil)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if runErr != nil {
		t.Fatalf("runCost table error: %v", runErr)
	}
	out := buf.String()
	if !strings.Contains(out, "prod") {
		t.Errorf("table output missing host 'prod': %s", out)
	}
}

func TestBuildCostEntries_WithMeta(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {
				Address: "1.2.3.4",
				Meta: map[string]any{
					"provider":     "hetzner",
					"plan":         "cx21",
					"currency":     "€",
					"cost_monthly": float64(9.99),
				},
			},
		},
	}
	t.Cleanup(func() { cfg = origCfg })

	entries := buildCostEntries([]string{"prod"})
	if len(entries) != 1 {
		t.Fatalf("buildCostEntries() = %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Host != "prod" {
		t.Errorf("Host = %q, want %q", e.Host, "prod")
	}
	if e.Provider != "hetzner" {
		t.Errorf("Provider = %q, want %q", e.Provider, "hetzner")
	}
	if e.Plan != "cx21" {
		t.Errorf("Plan = %q, want %q", e.Plan, "cx21")
	}
	if e.Currency != "€" {
		t.Errorf("Currency = %q, want %q", e.Currency, "€")
	}
	if e.Cost != 9.99 {
		t.Errorf("Cost = %v, want 9.99", e.Cost)
	}
}

func TestBuildCostEntries_NoMeta(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"dev": {Address: "5.6.7.8"},
		},
	}
	t.Cleanup(func() { cfg = origCfg })

	entries := buildCostEntries([]string{"dev"})
	if len(entries) != 1 {
		t.Fatalf("buildCostEntries() = %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Host != "dev" {
		t.Errorf("Host = %q, want %q", e.Host, "dev")
	}
	if e.Cost != 0 {
		t.Errorf("Cost = %v, want 0 (no meta)", e.Cost)
	}
}

func TestBuildCostEntries_Empty(t *testing.T) {
	origCfg := cfg
	cfg = &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"prod": {Address: "1.2.3.4"}},
	}
	t.Cleanup(func() { cfg = origCfg })

	entries := buildCostEntries([]string{})
	if len(entries) != 0 {
		t.Errorf("buildCostEntries([]) = %d entries, want 0", len(entries))
	}
}
