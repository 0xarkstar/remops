package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
)

// ContainerRow holds display data for one container in the table.
type ContainerRow struct {
	Name   string
	Image  string
	Status string
	State  string
}

// HostContainers is implemented by results that carry per-host container data.
type HostContainers interface {
	HostName() string
	ContainerRows() []ContainerRow
}

// TableFormatter outputs a Response as a human-readable table.
type TableFormatter struct{}

// Format writes the response as a table.
func (f *TableFormatter) Format(w io.Writer, resp *Response) error {
	if len(resp.Results) > 0 {
		if !formatContainerTable(w, resp.Results) {
			for _, result := range resp.Results {
				switch v := result.(type) {
				case map[string]any:
					if err := formatMap(w, v); err != nil {
						return err
					}
				case []any:
					if err := formatSlice(w, v); err != nil {
						return err
					}
				default:
					fmt.Fprintf(w, "%v\n", v)
				}
			}
		}
	}

	if len(resp.Failures) > 0 {
		red := color.New(color.FgRed).SprintFunc()
		fmt.Fprintln(w)
		fmt.Fprintln(w, red("Failures:"))
		for _, fail := range resp.Failures {
			fmt.Fprintf(w, "  %s [%s]: %s\n", red(fail.Host), fail.Code, fail.Message)
			if fail.Suggestion != "" {
				fmt.Fprintf(w, "    Suggestion: %s\n", fail.Suggestion)
			}
		}
	}

	if resp.Summary != nil {
		fmt.Fprintf(w, "\n%d/%d succeeded (%dms)\n",
			resp.Summary.Success, resp.Summary.Total, resp.Duration)
	}

	return nil
}

// formatContainerTable renders results as a column-aligned container table.
// Returns false if results are not HostContainers-shaped (caller falls back to formatMap/formatSlice).
func formatContainerTable(w io.Writer, results []any) bool {
	hostResults := make([]HostContainers, 0, len(results))
	for _, r := range results {
		hc, ok := r.(HostContainers)
		if !ok {
			return false
		}
		hostResults = append(hostResults, hc)
	}

	type tableRow struct {
		host      string
		container string
		image     string
		status    string
		state     string
	}

	var rows []tableRow
	for _, hr := range hostResults {
		containers := hr.ContainerRows()
		if len(containers) == 0 {
			rows = append(rows, tableRow{host: hr.HostName(), container: "(no containers)"})
		} else {
			for _, c := range containers {
				rows = append(rows, tableRow{
					host:      hr.HostName(),
					container: c.Name,
					image:     c.Image,
					status:    c.Status,
					state:     c.State,
				})
			}
		}
	}

	headers := []string{"HOST", "CONTAINER", "IMAGE", "STATUS", "STATE"}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		cols := []string{r.host, r.container, r.image, r.status, r.state}
		for i, c := range cols {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
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

	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	for _, r := range rows {
		stateStr := r.state
		switch strings.ToLower(r.state) {
		case "running":
			stateStr = green(r.state)
		case "exited", "dead", "removing":
			stateStr = red(r.state)
		default:
			if r.state != "" {
				stateStr = yellow(r.state)
			}
		}
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-*s  %s\n",
			widths[0], r.host,
			widths[1], r.container,
			widths[2], r.image,
			widths[3], r.status,
			stateStr,
		)
	}

	return true
}

func formatMap(w io.Writer, m map[string]any) error {
	maxKeyLen := 0
	for k := range m {
		if len(k) > maxKeyLen {
			maxKeyLen = len(k)
		}
	}

	for k, v := range m {
		padding := strings.Repeat(" ", maxKeyLen-len(k))
		fmt.Fprintf(w, "  %s%s  %v\n", k, padding, v)
	}
	return nil
}

func formatSlice(w io.Writer, s []any) error {
	for _, item := range s {
		if m, ok := item.(map[string]any); ok {
			if err := formatMap(w, m); err != nil {
				return err
			}
			fmt.Fprintln(w)
		} else {
			fmt.Fprintf(w, "  %v\n", item)
		}
	}
	return nil
}
