package output

import (
	"encoding/json"
	"io"
)

// JSONFormatter outputs a Response as indented JSON.
type JSONFormatter struct{}

// Format writes the response as JSON.
func (f *JSONFormatter) Format(w io.Writer, resp *Response) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}
