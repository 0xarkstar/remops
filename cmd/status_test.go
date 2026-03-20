package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/docker"
	"github.com/0xarkstar/remops/internal/output"
	"github.com/0xarkstar/remops/internal/transport"
)

func TestHostResult_HostName(t *testing.T) {
	hr := &hostResult{Host: "prod"}
	if got := hr.HostName(); got != "prod" {
		t.Errorf("HostName() = %q, want %q", got, "prod")
	}
}

func TestHostResult_ContainerRows_WithContainers(t *testing.T) {
	hr := &hostResult{
		Host: "prod",
		Containers: []docker.ContainerInfo{
			{Name: "app", Image: "nginx:latest", Status: "Up 2 hours", State: "running"},
			{Name: "db", Image: "postgres:15", Status: "Up 1 day", State: "running"},
		},
	}

	rows := hr.ContainerRows()
	if len(rows) != 2 {
		t.Fatalf("ContainerRows() returned %d rows, want 2", len(rows))
	}

	want := []output.ContainerRow{
		{Name: "app", Image: "nginx:latest", Status: "Up 2 hours", State: "running"},
		{Name: "db", Image: "postgres:15", Status: "Up 1 day", State: "running"},
	}
	for i, row := range rows {
		if row.Name != want[i].Name {
			t.Errorf("row[%d].Name = %q, want %q", i, row.Name, want[i].Name)
		}
		if row.Image != want[i].Image {
			t.Errorf("row[%d].Image = %q, want %q", i, row.Image, want[i].Image)
		}
		if row.Status != want[i].Status {
			t.Errorf("row[%d].Status = %q, want %q", i, row.Status, want[i].Status)
		}
		if row.State != want[i].State {
			t.Errorf("row[%d].State = %q, want %q", i, row.State, want[i].State)
		}
	}
}

func TestHostResult_ContainerRows_Empty(t *testing.T) {
	hr := &hostResult{
		Host:       "prod",
		Containers: []docker.ContainerInfo{},
	}

	rows := hr.ContainerRows()
	if len(rows) != 0 {
		t.Errorf("ContainerRows() returned %d rows, want 0", len(rows))
	}
}

func TestHostResult_ContainerRows_NilContainers(t *testing.T) {
	hr := &hostResult{
		Host:       "prod",
		Containers: nil,
	}

	rows := hr.ContainerRows()
	if len(rows) != 0 {
		t.Errorf("ContainerRows() with nil containers returned %d rows, want 0", len(rows))
	}
}

func TestGatherHostData_Success(t *testing.T) {
	// Build mock responses for each command SystemInfo + ListContainers make.
	// SystemInfo calls: uname -s, then Linux-specific: nproc, free -m, df -h /, uptime, docker --version
	// ListContainers calls: docker ps --format ...
	tr := &mockTransport{
		execFunc: func(host, cmd string) (transport.ExecResult, error) {
			switch {
			case strings.HasPrefix(cmd, "uname"):
				return transport.ExecResult{Stdout: "Linux\n", ExitCode: 0}, nil
			case strings.HasPrefix(cmd, "nproc"):
				return transport.ExecResult{Stdout: "4\n", ExitCode: 0}, nil
			case strings.HasPrefix(cmd, "free"):
				return transport.ExecResult{Stdout: "Mem: 8192 2048 6144\n", ExitCode: 0}, nil
			case strings.HasPrefix(cmd, "df"):
				return transport.ExecResult{Stdout: "Filesystem Size Used Avail Use% Mounted\n/dev/sda1 100G 40G 60G 40% /\n", ExitCode: 0}, nil
			case strings.HasPrefix(cmd, "uptime"):
				return transport.ExecResult{Stdout: "up 2 days\n", ExitCode: 0}, nil
			case strings.HasPrefix(cmd, "docker --version"):
				return transport.ExecResult{Stdout: "Docker version 24.0.0\n", ExitCode: 0}, nil
			case strings.HasPrefix(cmd, "docker ps"):
				return transport.ExecResult{Stdout: "", ExitCode: 0}, nil
			default:
				return transport.ExecResult{ExitCode: 0}, nil
			}
		},
	}

	dc := docker.NewDockerClient(tr)
	result, err := gatherHostData(context.Background(), dc, "prod")
	if err != nil {
		t.Fatalf("gatherHostData() error: %v", err)
	}
	if result.Host != "prod" {
		t.Errorf("Host = %q, want %q", result.Host, "prod")
	}
	if result.Info == nil {
		t.Error("Info should not be nil")
	}
}

func TestGatherHostData_ExecError(t *testing.T) {
	tr := &mockTransport{
		execFunc: func(host, cmd string) (transport.ExecResult, error) {
			if strings.HasPrefix(cmd, "uname") {
				return transport.ExecResult{}, context.DeadlineExceeded
			}
			return transport.ExecResult{ExitCode: 0}, nil
		},
	}

	dc := docker.NewDockerClient(tr)
	_, err := gatherHostData(context.Background(), dc, "prod")
	if err == nil {
		t.Error("expected error when exec fails")
	}
}
