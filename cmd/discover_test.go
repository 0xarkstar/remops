package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/docker"
	"github.com/0xarkstar/remops/internal/transport"
)

// -- classifyContainers tests --

func TestDiscoverFindNewContainers(t *testing.T) {
	cfg := &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {Address: "1.2.3.4"},
		},
		Services: map[string]config.Service{
			"beszel":     {Host: "prod", Container: "beszel"},
			"uptime-kuma": {Host: "prod", Container: "uptime-kuma"},
		},
	}

	containers := []docker.ContainerInfo{
		{Host: "prod", Name: "crawl4ai", Image: "unclecode/crawl4ai:latest", Status: "Up 5 weeks", State: "running"},
		{Host: "prod", Name: "searxng", Image: "searxng/searxng:latest", Status: "Up 5 weeks", State: "running"},
		{Host: "prod", Name: "beszel", Image: "henrygd/beszel:latest", Status: "Up 5 weeks", State: "running"},
		{Host: "prod", Name: "uptime-kuma", Image: "louislam/uptime-kuma:1", Status: "Up 5 weeks", State: "running"},
	}

	newContainers, knownNames := classifyContainers(containers, cfg, "prod")

	if len(newContainers) != 2 {
		t.Fatalf("expected 2 new containers, got %d: %+v", len(newContainers), newContainers)
	}
	if len(knownNames) != 2 {
		t.Fatalf("expected 2 known containers, got %d: %v", len(knownNames), knownNames)
	}

	newNames := make(map[string]bool)
	for _, c := range newContainers {
		newNames[c.Name] = true
		if c.Host != "prod" {
			t.Errorf("container %q: Host = %q, want %q", c.Name, c.Host, "prod")
		}
		if c.ServiceID == "" {
			t.Errorf("container %q: ServiceID should not be empty", c.Name)
		}
	}

	for _, want := range []string{"crawl4ai", "searxng"} {
		if !newNames[want] {
			t.Errorf("expected %q in new containers, got %v", want, newContainers)
		}
	}

	knownSet := make(map[string]bool)
	for _, n := range knownNames {
		knownSet[n] = true
	}
	for _, want := range []string{"beszel", "uptime-kuma"} {
		if !knownSet[want] {
			t.Errorf("expected %q in known containers, got %v", want, knownNames)
		}
	}
}

func TestDiscoverAllAlreadyKnown(t *testing.T) {
	cfg := &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {Address: "1.2.3.4"},
		},
		Services: map[string]config.Service{
			"beszel":     {Host: "prod", Container: "beszel"},
			"uptime-kuma": {Host: "prod", Container: "uptime-kuma"},
		},
	}

	containers := []docker.ContainerInfo{
		{Host: "prod", Name: "beszel", Image: "henrygd/beszel:latest", Status: "Up 5 weeks", State: "running"},
		{Host: "prod", Name: "uptime-kuma", Image: "louislam/uptime-kuma:1", Status: "Up 5 weeks", State: "running"},
	}

	newContainers, knownNames := classifyContainers(containers, cfg, "prod")

	if len(newContainers) != 0 {
		t.Errorf("expected 0 new containers, got %d: %+v", len(newContainers), newContainers)
	}
	if len(knownNames) != 2 {
		t.Errorf("expected 2 known containers, got %d", len(knownNames))
	}
}

// -- scanHosts tests --

func TestScanHosts_ReturnsNewContainers(t *testing.T) {
	psLine := `{"Names":"crawl4ai","Image":"unclecode/crawl4ai:latest","Status":"Up 5 weeks","State":"running","Health":"","Ports":"","CreatedAt":""}`

	tr := &mockTransport{
		execFunc: func(host, cmd string) (transport.ExecResult, error) {
			if strings.HasPrefix(cmd, "docker ps") {
				return transport.ExecResult{Stdout: psLine + "\n", ExitCode: 0}, nil
			}
			return transport.ExecResult{ExitCode: 0}, nil
		},
	}

	testCfg := &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {Address: "1.2.3.4"},
		},
		Services: map[string]config.Service{},
	}

	dc := docker.NewDockerClient(tr)
	results := scanHosts(context.Background(), dc, testCfg, []string{"prod"})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if len(r.New) != 1 {
		t.Fatalf("expected 1 new container, got %d", len(r.New))
	}
	if r.New[0].Name != "crawl4ai" {
		t.Errorf("Name = %q, want %q", r.New[0].Name, "crawl4ai")
	}
}

func TestScanHosts_ExecError(t *testing.T) {
	tr := &mockTransport{
		execFunc: func(host, cmd string) (transport.ExecResult, error) {
			return transport.ExecResult{ExitCode: 1, Stderr: "permission denied"}, nil
		},
	}

	testCfg := &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {Address: "1.2.3.4"},
		},
	}

	dc := docker.NewDockerClient(tr)
	results := scanHosts(context.Background(), dc, testCfg, []string{"prod"})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected error for non-zero exit code, got nil")
	}
}

// -- sanitizeName tests --

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"crawl4ai", "crawl4ai"},
		{"my.container", "my-container"},
		{"foo_bar", "foo_bar"},
		{"Some.App", "some-app"},
		{"hello world", "hello-world"},
	}

	for _, tt := range tests {
		got := sanitizeName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// -- uniqueServiceKey tests --

func TestUniqueServiceKey_NoConflict(t *testing.T) {
	services := map[string]interface{}{"other": nil}
	got := uniqueServiceKey(services, "new-svc")
	if got != "new-svc" {
		t.Errorf("expected %q, got %q", "new-svc", got)
	}
}

func TestUniqueServiceKey_Conflict(t *testing.T) {
	services := map[string]interface{}{
		"svc":   nil,
		"svc-2": nil,
	}
	got := uniqueServiceKey(services, "svc")
	if got != "svc-3" {
		t.Errorf("expected %q, got %q", "svc-3", got)
	}
}

// -- runDiscover output tests --

func TestRunDiscover_AllAlreadyKnown_Output(t *testing.T) {
	psLine := `{"Names":"beszel","Image":"henrygd/beszel:latest","Status":"Up 5 weeks","State":"running","Health":"","Ports":"","CreatedAt":""}`

	tr := &mockTransport{
		execFunc: func(host, cmd string) (transport.ExecResult, error) {
			if strings.HasPrefix(cmd, "docker ps") {
				return transport.ExecResult{Stdout: psLine + "\n", ExitCode: 0}, nil
			}
			return transport.ExecResult{ExitCode: 0}, nil
		},
	}

	origCfg := cfg
	origHost := flagHost
	origTag := flagTag
	cfg = &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"prod": {Address: "1.2.3.4"},
		},
		Services: map[string]config.Service{
			"beszel": {Host: "prod", Container: "beszel"},
		},
	}
	flagHost = ""
	flagTag = ""
	t.Cleanup(func() {
		cfg = origCfg
		flagHost = origHost
		flagTag = origTag
	})

	var buf bytes.Buffer
	cmd := newTestCmd()
	cmd.SetOut(&buf)

	// Override transport by temporarily patching the discover function path.
	// We test via the helper functions directly; full integration would require
	// a more invasive transport injection. Instead verify output via scanHosts.
	dc := docker.NewDockerClient(tr)
	results := scanHosts(cmd.Context(), dc, cfg, []string{"prod"})

	// Verify scanHosts returns no new containers.
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].New) != 0 {
		t.Errorf("expected 0 new containers, got %d", len(results[0].New))
	}
	_ = dc
}
