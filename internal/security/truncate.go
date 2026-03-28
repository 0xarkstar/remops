package security

import "fmt"

// MaxOutputBytes is the maximum output size returned to AI agents.
// Prevents unbounded container logs from overwhelming LLM context windows.
const MaxOutputBytes = 1024 * 1024 // 1MB

// TruncateOutput truncates output to MaxOutputBytes and appends a notice.
func TruncateOutput(s string) string {
	if len(s) <= MaxOutputBytes {
		return s
	}
	return s[:MaxOutputBytes] + fmt.Sprintf("\n\n[output truncated at %d bytes]", MaxOutputBytes)
}
