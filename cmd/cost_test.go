package cmd

import (
	"bytes"
	"strings"
	"testing"
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
