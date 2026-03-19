package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
)

// TableFormatter outputs a Response as a human-readable table.
type TableFormatter struct{}

// Format writes the response as a table.
func (f *TableFormatter) Format(w io.Writer, resp *Response) error {
	if len(resp.Results) > 0 {
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
