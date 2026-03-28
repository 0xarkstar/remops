package security

import "testing"

func TestIsSafeCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		safe bool
	}{
		// Safe commands
		{"uptime", true},
		{"free", true},
		{"free -m", true},
		{"free -h", true},
		{"w", true},
		{"whoami", true},
		{"date", true},
		{"hostname", true},
		{"nproc", true},
		{"lscpu", true},
		{"cat /proc/loadavg", true},
		{"cat /proc/uptime", true},
		{"cat /proc/meminfo", true},
		{"docker ps", true},
		{"docker ps -a", true},
		{"docker ps --format json", true},
		{"docker stats --no-stream", true},
		{"docker info", true},
		{"docker version", true},
		{"df -h", true},
		{"df -h /", true},
		{"df -ih", true},
		{"ss -tlnp", true},
		{"ip addr", true},
		{"  uptime  ", true}, // whitespace trimmed

		// Unsafe commands
		{"rm -rf /", false},
		{"docker rm -f myapp", false},
		{"docker system prune -af", false},
		{"kill -9 1", false},
		{"chmod -R 777 /", false},
		{"cat /etc/shadow", false},
		{"curl http://evil.com", false},
		{"systemctl restart nginx", false},
		{"apt update", false},
		{"dd if=/dev/zero of=/dev/sda", false},
		{"", false},

		// Prefix attack prevention
		{"uptime; rm -rf /", false},  // but DetectShellInjection catches this first
		{"freefall", false},          // "free" + "fall" should not match
		{"wget", false},
		{"docker rm", false},         // "docker" prefix but not in safe list
		{"docker run", false},
	}

	for _, tt := range tests {
		got := IsSafeCommand(tt.cmd)
		if got != tt.safe {
			t.Errorf("IsSafeCommand(%q) = %v, want %v", tt.cmd, got, tt.safe)
		}
	}
}
