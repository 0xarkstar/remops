package output

import (
	"io"
	"os"

	"golang.org/x/term"
)

// Format represents the output format.
type Format string

const (
	FormatJSON  Format = "json"
	FormatTable Format = "table"
	FormatAuto  Format = "auto"
)

// Formatter renders a Response to a writer.
type Formatter interface {
	Format(w io.Writer, resp *Response) error
}

// NewFormatter creates a formatter based on the requested format.
func NewFormatter(format Format) Formatter {
	resolved := resolveFormat(format)
	switch resolved {
	case FormatJSON:
		return &JSONFormatter{}
	case FormatTable:
		return &TableFormatter{}
	default:
		return &JSONFormatter{}
	}
}

// resolveFormat determines the actual format when "auto" is specified.
func resolveFormat(format Format) Format {
	if format != FormatAuto {
		return format
	}
	if IsTTY() {
		return FormatTable
	}
	return FormatJSON
}

// IsTTY returns true if stdout is connected to a terminal.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
