package docker

import "testing"

func TestParseDarwinVMStatPage(t *testing.T) {
	output := `Mach Virtual Memory Statistics: (page size of 4096 bytes)
Pages free:                             12345.
Pages active:                           67890.
Pages inactive:                         11111.
Pages speculative:                        5000.
Pages throttled:                             0.
Pages wired down:                        22222.
`
	tests := []struct {
		key  string
		want int64
	}{
		{"Pages free", 12345},
		{"Pages inactive", 11111},
		{"Pages speculative", 5000},
		{"Pages active", 67890},
		{"Pages missing", 0},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			got := parseDarwinVMStatPage(output, tc.key)
			if got != tc.want {
				t.Errorf("parseDarwinVMStatPage(%q): want %d, got %d", tc.key, tc.want, got)
			}
		})
	}
}

func TestParseLinuxFreeOutput(t *testing.T) {
	output := `              total        used        free      shared  buff/cache   available
Mem:           7953        1234        5000         100        1500        6000
Swap:          2047           0        2047
`
	totalMB, usedMB := parseLinuxFreeOutput(output)
	if totalMB != 7953 {
		t.Errorf("totalMB: want 7953, got %d", totalMB)
	}
	if usedMB != 1234 {
		t.Errorf("usedMB: want 1234, got %d", usedMB)
	}
}

func TestParseLinuxFreeOutputEmpty(t *testing.T) {
	totalMB, usedMB := parseLinuxFreeOutput("")
	if totalMB != 0 || usedMB != 0 {
		t.Errorf("empty input: want 0,0, got %d,%d", totalMB, usedMB)
	}
}

func TestParseDFOutput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantTotal   string
		wantUsed    string
		wantPercent string
	}{
		{
			name: "linux",
			input: `Filesystem      Size  Used Avail Use% Mounted on
/dev/sda1        50G   20G   28G  42% /
`,
			wantTotal: "50G", wantUsed: "20G", wantPercent: "42%",
		},
		{
			name: "darwin",
			input: `Filesystem     Size   Used  Avail Capacity iused      ifree %iused  Mounted on
/dev/disk3s5  228Gi  100Gi  120Gi    46%  553613 1259798027    0%   /
`,
			wantTotal: "228Gi", wantUsed: "100Gi", wantPercent: "46%",
		},
		{
			name:        "empty",
			input:       "",
			wantTotal:   "",
			wantUsed:    "",
			wantPercent: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			total, used, percent := parseDFOutput(tc.input)
			if total != tc.wantTotal {
				t.Errorf("total: want %q, got %q", tc.wantTotal, total)
			}
			if used != tc.wantUsed {
				t.Errorf("used: want %q, got %q", tc.wantUsed, used)
			}
			if percent != tc.wantPercent {
				t.Errorf("percent: want %q, got %q", tc.wantPercent, percent)
			}
		})
	}
}
