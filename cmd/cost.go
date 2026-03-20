package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/output"
	"github.com/0xarkstar/remops/internal/security"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// hostCost holds cost metadata for a host.
type hostCost struct {
	Host     string  `json:"host"`
	Provider string  `json:"provider,omitempty"`
	Plan     string  `json:"plan,omitempty"`
	Cost     float64 `json:"cost_monthly"`
	Currency string  `json:"currency,omitempty"`
}

var costCmd = &cobra.Command{
	Use:         "cost",
	Short:       "Show monthly cost breakdown for configured hosts",
	Annotations: map[string]string{"permission": "viewer"},
	RunE:        runCost,
}

func init() {
	rootCmd.AddCommand(costCmd)
}

func runCost(cmd *cobra.Command, args []string) error {
	if err := security.CheckPermission(currentProfileLevel(), config.LevelViewer); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitPermissionDenied)
	}

	hosts := resolveHosts()
	entries := buildCostEntries(hosts)

	if flagFormat == "json" || (!output.IsTTY() && flagFormat == "auto") {
		return printCostJSON(os.Stdout, entries)
	}
	printCostTable(os.Stdout, entries)
	return nil
}

func buildCostEntries(hosts []string) []hostCost {
	entries := make([]hostCost, 0, len(hosts))
	for _, name := range hosts {
		h := cfg.Hosts[name]
		entry := hostCost{Host: name}
		if h.Meta != nil {
			entry.Provider = metaString(h.Meta, "provider")
			entry.Plan = metaString(h.Meta, "plan")
			entry.Currency = metaString(h.Meta, "currency")
			entry.Cost = metaFloat(h.Meta, "cost_monthly")
		}
		entries = append(entries, entry)
	}
	return entries
}

func printCostJSON(w io.Writer, entries []hostCost) error {
	type resp struct {
		Hosts []hostCost `json:"hosts"`
		Total float64    `json:"total"`
	}
	var total float64
	for _, e := range entries {
		total += e.Cost
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp{Hosts: entries, Total: total})
}

func printCostTable(w io.Writer, entries []hostCost) {
	headers := []string{"HOST", "PROVIDER", "PLAN", "COST"}
	widths := []int{len("HOST"), len("PROVIDER"), len("PLAN"), len("COST")}

	costStrs := make([]string, len(entries))
	for i, e := range entries {
		costStrs[i] = formatCostStr(e)
	}

	for i, e := range entries {
		if len(e.Host) > widths[0] {
			widths[0] = len(e.Host)
		}
		if len(e.Provider) > widths[1] {
			widths[1] = len(e.Provider)
		}
		if len(e.Plan) > widths[2] {
			widths[2] = len(e.Plan)
		}
		if len(costStrs[i]) > widths[3] {
			widths[3] = len(costStrs[i])
		}
	}

	bold := color.New(color.Bold)
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(w, "  ")
		}
		bold.Fprint(w, h)
		fmt.Fprint(w, strings.Repeat(" ", widths[i]-len(h)))
	}
	fmt.Fprintln(w)

	var total float64
	var totalCurrency string
	for i, e := range entries {
		total += e.Cost
		if e.Currency != "" {
			totalCurrency = e.Currency
		}
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %s\n",
			widths[0], e.Host,
			widths[1], e.Provider,
			widths[2], e.Plan,
			costStrs[i],
		)
	}

	totalStr := fmt.Sprintf("%s%.2f", totalCurrency, total)
	fmt.Fprintf(w, "%-*s  %-*s  %-*s  %s\n",
		widths[0], "",
		widths[1], "",
		widths[2], "TOTAL",
		totalStr,
	)
}

func formatCostStr(e hostCost) string {
	if e.Cost == 0 {
		return "-"
	}
	return fmt.Sprintf("%s%.2f", e.Currency, e.Cost)
}

func metaString(meta map[string]any, key string) string {
	v, ok := meta[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func metaFloat(meta map[string]any, key string) float64 {
	v, ok := meta[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case float32:
		return float64(n)
	}
	return 0
}
