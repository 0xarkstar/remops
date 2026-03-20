package docker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/transport"
)

func makeExecMap(responses map[string]transport.ExecResult) func(context.Context, string, string) (transport.ExecResult, error) {
	return func(_ context.Context, _ string, cmd string) (transport.ExecResult, error) {
		for key, result := range responses {
			if strings.HasPrefix(cmd, key) {
				return result, nil
			}
		}
		return transport.ExecResult{ExitCode: 0}, nil
	}
}

func TestSystemInfoLinux(t *testing.T) {
	responses := map[string]transport.ExecResult{
		"uname -s":         {Stdout: "Linux\n", ExitCode: 0},
		"nproc":            {Stdout: "4\n", ExitCode: 0},
		"free -m":          {Stdout: "              total        used        free\nMem:           7953        1234        6000\nSwap:          2047           0        2047\n", ExitCode: 0},
		"df -h /":          {Stdout: "Filesystem  Size  Used Avail Use% Mounted on\n/dev/sda1   50G   20G   28G  42% /\n", ExitCode: 0},
		"uptime":           {Stdout: " 10:00  up 2 days\n", ExitCode: 0},
		"docker --version": {Stdout: "Docker version 24.0.0, build abc123\n", ExitCode: 0},
	}
	mt := &mockTransport{execFunc: makeExecMap(responses)}

	info, err := NewDockerClient(mt).SystemInfo(context.Background(), "host1")
	if err != nil {
		t.Fatalf("SystemInfo: unexpected error: %v", err)
	}
	if info.OS != "Linux" {
		t.Errorf("OS: want Linux, got %s", info.OS)
	}
	if info.CPUs != 4 {
		t.Errorf("CPUs: want 4, got %d", info.CPUs)
	}
	if info.MemTotalMB != 7953 {
		t.Errorf("MemTotalMB: want 7953, got %d", info.MemTotalMB)
	}
	if info.MemUsedMB != 1234 {
		t.Errorf("MemUsedMB: want 1234, got %d", info.MemUsedMB)
	}
	if info.DiskTotal != "50G" {
		t.Errorf("DiskTotal: want 50G, got %s", info.DiskTotal)
	}
	if info.DockerVersion == "" {
		t.Error("expected DockerVersion to be set")
	}
	if info.Host != "host1" {
		t.Errorf("Host: want host1, got %s", info.Host)
	}
}

func TestSystemInfoDarwin(t *testing.T) {
	vmStatOutput := `Mach Virtual Memory Statistics: (page size of 4096 bytes)
Pages free:                            100000.
Pages active:                          200000.
Pages inactive:                         50000.
Pages speculative:                      10000.
Pages wired down:                       80000.
`
	responses := map[string]transport.ExecResult{
		"uname -s":           {Stdout: "Darwin\n", ExitCode: 0},
		"sysctl -n hw.ncpu":  {Stdout: "8\n", ExitCode: 0},
		"sysctl -n hw.memsz": {Stdout: "17179869184\n", ExitCode: 0}, // 16GB
		"vm_stat":            {Stdout: vmStatOutput, ExitCode: 0},
		"df -h /":            {Stdout: "Filesystem     Size   Used  Avail Capacity Mounted on\n/dev/disk3s5  228Gi  100Gi  120Gi    46% /\n", ExitCode: 0},
		"uptime":             {Stdout: "10:00  up 1 day\n", ExitCode: 0},
		"docker --version":   {Stdout: "Docker version 24.0.0\n", ExitCode: 0},
	}
	mt := &mockTransport{execFunc: makeExecMap(responses)}

	info, err := NewDockerClient(mt).SystemInfo(context.Background(), "mac1")
	if err != nil {
		t.Fatalf("SystemInfo Darwin: unexpected error: %v", err)
	}
	if info.OS != "Darwin" {
		t.Errorf("OS: want Darwin, got %s", info.OS)
	}
	if info.CPUs != 8 {
		t.Errorf("CPUs: want 8, got %d", info.CPUs)
	}
}

func TestSystemInfoUnameError(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, cmd string) (transport.ExecResult, error) {
			if strings.HasPrefix(cmd, "uname") {
				return transport.ExecResult{}, errors.New("fake exec error")
			}
			return transport.ExecResult{ExitCode: 0}, nil
		},
	}
	_, err := NewDockerClient(mt).SystemInfo(context.Background(), "host1")
	if err == nil {
		t.Error("expected error when uname fails, got nil")
	}
}
