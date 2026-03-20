package docker

import (
	"context"
	"testing"

	"github.com/0xarkstar/remops/internal/transport"
)

const psNDJSON = `{"Names":"myapp","Image":"nginx:latest","Status":"Up 2 hours","State":"running","Health":"healthy","Ports":"80/tcp","CreatedAt":"2024-01-01 10:00:00 +0000 UTC"}
{"Names":"db","Image":"postgres:15","Status":"Up 1 hour","State":"running","Health":"","Ports":"5432/tcp","CreatedAt":"2024-01-01 09:00:00 +0000 UTC"}
`

func TestListContainers(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: psNDJSON, ExitCode: 0}, nil
		},
	}
	client := NewDockerClient(mt)
	containers, err := client.ListContainers(context.Background(), "web1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("want 2 containers, got %d", len(containers))
	}
	if containers[0].Name != "myapp" {
		t.Errorf("container[0].Name: want myapp, got %s", containers[0].Name)
	}
	if containers[0].Host != "web1" {
		t.Errorf("container[0].Host: want web1, got %s", containers[0].Host)
	}
	if containers[0].Image != "nginx:latest" {
		t.Errorf("container[0].Image: want nginx:latest, got %s", containers[0].Image)
	}
	if containers[1].Name != "db" {
		t.Errorf("container[1].Name: want db, got %s", containers[1].Name)
	}
}

func TestListContainersEmpty(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "", ExitCode: 0}, nil
		},
	}
	client := NewDockerClient(mt)
	containers, err := client.ListContainers(context.Background(), "web1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 0 {
		t.Errorf("want 0 containers, got %d", len(containers))
	}
}

func TestListContainersExitError(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{
				Stdout:   "",
				Stderr:   "permission denied",
				ExitCode: 1,
			}, nil
		},
	}
	client := NewDockerClient(mt)
	_, err := client.ListContainers(context.Background(), "web1")
	if err == nil {
		t.Fatal("expected error for non-zero exit code, got nil")
	}
}

func TestListContainersParseError(t *testing.T) {
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: "not valid json{{{", ExitCode: 0}, nil
		},
	}
	client := NewDockerClient(mt)
	containers, err := client.ListContainers(context.Background(), "web1")
	if err != nil {
		t.Fatalf("unexpected error for malformed line: %v", err)
	}
	if len(containers) != 0 {
		t.Errorf("want 0 containers for all-bad input, got %d", len(containers))
	}
}

func TestListContainers_PartialParsing(t *testing.T) {
	mixed := `{"Names":"myapp","Image":"nginx:latest","Status":"Up 2 hours","State":"running","Health":"healthy","Ports":"80/tcp","CreatedAt":"2024-01-01 10:00:00 +0000 UTC"}
not valid json at all
`
	mt := &mockTransport{
		execFunc: func(_ context.Context, _, _ string) (transport.ExecResult, error) {
			return transport.ExecResult{Stdout: mixed, ExitCode: 0}, nil
		},
	}
	client := NewDockerClient(mt)
	containers, err := client.ListContainers(context.Background(), "web1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("want 1 container, got %d", len(containers))
	}
	if containers[0].Name != "myapp" {
		t.Errorf("want myapp, got %s", containers[0].Name)
	}
}
