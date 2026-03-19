package security

import (
	"regexp"
)

// llmDirectiveRe matches LLM prompt injection patterns that could hijack tool output parsing.
var llmDirectiveRe = regexp.MustCompile(
	`(?i)(IMPORTANT:|SYSTEM:|INSTRUCTION:|IGNORE PREVIOUS|<\/?system>|<tool_call>|<function_call>|\[INST\]|\[\/INST\]|<<SYS>>)`,
)

// SanitizeOutput strips LLM directive patterns from remote command output.
func SanitizeOutput(output string) string {
	return llmDirectiveRe.ReplaceAllString(output, "[REDACTED]")
}
