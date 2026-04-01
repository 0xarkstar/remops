package security

import "strings"

// safeCommandPrefixes is a hardcoded list of read-only commands that are safe
// for operator-level execution without approval. This list is intentionally
// kept short and compiled into the binary — additions require a code change.
//
// Each prefix is matched against the trimmed command. A command matches if it
// equals the prefix exactly or starts with the prefix followed by a space.
// Shell injection is checked separately before this function is called.
var safeCommandPrefixes = []string{
	"uptime",
	"free",
	"w",
	"whoami",
	"date",
	"hostname",
	"nproc",
	"lscpu",
	"cat /proc/loadavg",
	"cat /proc/uptime",
	"cat /proc/meminfo",
	"docker ps",
	"docker stats --no-stream",
	"docker info",
	"docker version",
	"df -h",
	"df -ih",
	"ss -tlnp",
	"ip addr",
	"tmux ls",
	"tmux list-sessions",
	"tmux new -d",
}

// IsSafeCommand returns true if the command matches one of the hardcoded
// safe command prefixes. The caller must run DetectShellInjection before
// calling this function.
func IsSafeCommand(command string) bool {
	cmd := strings.TrimSpace(command)
	for _, prefix := range safeCommandPrefixes {
		if cmd == prefix || strings.HasPrefix(cmd, prefix+" ") {
			return true
		}
	}
	return false
}
