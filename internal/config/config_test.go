package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const validYAML = `version: 1
hosts:
  web1:
    address: 192.168.1.1
    tags: [prod, web]
  db1:
    address: 192.168.1.2
    tags: [prod, db]
services:
  myapp:
    host: web1
    container: myapp_container
    tags: [prod]
profiles:
  default:
    level: viewer
  ops:
    level: operator
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "remops.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParse(t *testing.T) {
	cfg, err := LoadFrom(writeConfig(t, validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Hosts) != 2 {
		t.Errorf("hosts: want 2, got %d", len(cfg.Hosts))
	}
	if cfg.Hosts["web1"].Address != "192.168.1.1" {
		t.Errorf("web1 address: want 192.168.1.1, got %s", cfg.Hosts["web1"].Address)
	}
	if len(cfg.Services) != 1 {
		t.Errorf("services: want 1, got %d", len(cfg.Services))
	}
	if cfg.Services["myapp"].Container != "myapp_container" {
		t.Errorf("myapp container: want myapp_container, got %s", cfg.Services["myapp"].Container)
	}
	if cfg.Profiles["default"].Level != "viewer" {
		t.Errorf("default profile level: want viewer, got %s", cfg.Profiles["default"].Level)
	}
	if cfg.Profiles["ops"].Level != "operator" {
		t.Errorf("ops profile level: want operator, got %s", cfg.Profiles["ops"].Level)
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "wrong version",
			yaml:    "version: 2\nhosts:\n  h1:\n    address: 1.2.3.4\n",
			wantErr: "unsupported config version",
		},
		{
			name:    "no hosts",
			yaml:    "version: 1\n",
			wantErr: "at least one host must be defined",
		},
		{
			name:    "missing host address",
			yaml:    "version: 1\nhosts:\n  h1:\n    user: root\n",
			wantErr: "address is required",
		},
		{
			name: "service references unknown host",
			yaml: "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\nservices:\n  svc1:\n    host: unknown\n    container: c1\n",
			wantErr: "unknown host",
		},
		{
			name: "service missing container",
			yaml: "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\nservices:\n  svc1:\n    host: h1\n",
			wantErr: "container is required",
		},
		{
			name: "invalid profile level",
			yaml: "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\nprofiles:\n  bad:\n    level: superuser\n",
			wantErr: "invalid level",
		},
		{
			name:    "telegram approval empty bot_token",
			yaml:    "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\napproval:\n  method: telegram\n  bot_token: \"\"\n  chat_id: \"123\"\n",
			wantErr: "non-empty bot_token",
		},
		{
			name:    "telegram approval empty chat_id",
			yaml:    "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\napproval:\n  method: telegram\n  bot_token: \"tok123\"\n  chat_id: \"\"\n",
			wantErr: "non-empty chat_id",
		},
		{
			name:    "db missing engine",
			yaml:    "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\nservices:\n  svc1:\n    host: h1\n    container: c1\n    db:\n      user: admin\n      database: mydb\n",
			wantErr: "db.engine is required",
		},
		{
			name:    "db invalid engine",
			yaml:    "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\nservices:\n  svc1:\n    host: h1\n    container: c1\n    db:\n      engine: sqlite\n      user: admin\n      database: mydb\n",
			wantErr: "unsupported db.engine",
		},
		{
			name:    "db missing user",
			yaml:    "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\nservices:\n  svc1:\n    host: h1\n    container: c1\n    db:\n      engine: postgresql\n      database: mydb\n",
			wantErr: "db.user is required",
		},
		{
			name:    "db missing database",
			yaml:    "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\nservices:\n  svc1:\n    host: h1\n    container: c1\n    db:\n      engine: mysql\n      user: root\n",
			wantErr: "db.database is required",
		},
		{
			name:    "stack missing host",
			yaml:    "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\nstacks:\n  mystack:\n    path: /home/user/app\n",
			wantErr: "host is required",
		},
		{
			name:    "stack unknown host",
			yaml:    "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\nstacks:\n  mystack:\n    host: unknown\n    path: /home/user/app\n",
			wantErr: "unknown host",
		},
		{
			name:    "stack missing path",
			yaml:    "version: 1\nhosts:\n  h1:\n    address: 1.2.3.4\nstacks:\n  mystack:\n    host: h1\n",
			wantErr: "path is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadFrom(writeConfig(t, tc.yaml))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestExpandEnvVars(t *testing.T) {
	t.Setenv("TEST_REMOPS_ADDR", "10.0.0.1")
	result := expandEnvVars("address: ${TEST_REMOPS_ADDR}")
	if result != "address: 10.0.0.1" {
		t.Errorf("want 'address: 10.0.0.1', got %q", result)
	}
}

func TestExpandEnvVarsDefault(t *testing.T) {
	os.Unsetenv("TEST_REMOPS_UNSET")
	result := expandEnvVars("val: ${TEST_REMOPS_UNSET:-fallback}")
	if result != "val: fallback" {
		t.Errorf("want 'val: fallback', got %q", result)
	}
}

func TestExpandEnvVarsMissing(t *testing.T) {
	os.Unsetenv("TEST_REMOPS_UNSET2")
	result := expandEnvVars("val: ${TEST_REMOPS_UNSET2}")
	if result != "val: ${TEST_REMOPS_UNSET2}" {
		t.Errorf("missing var without default should be unexpanded, got %q", result)
	}
}

func TestHostsByTag(t *testing.T) {
	cfg, err := LoadFrom(writeConfig(t, validYAML))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		tag  string
		want []string
	}{
		{"prod", []string{"web1", "db1"}},
		{"web", []string{"web1"}},
		{"db", []string{"db1"}},
		{"nope", []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.tag, func(t *testing.T) {
			got := cfg.HostsByTag(tc.tag)
			sort.Strings(got)
			want := make([]string, len(tc.want))
			copy(want, tc.want)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Errorf("HostsByTag(%q): want %v, got %v", tc.tag, want, got)
				return
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("HostsByTag(%q)[%d]: want %q, got %q", tc.tag, i, want[i], got[i])
				}
			}
		})
	}
}

func TestServicesByTag(t *testing.T) {
	cfg, err := LoadFrom(writeConfig(t, validYAML))
	if err != nil {
		t.Fatal(err)
	}

	got := cfg.ServicesByTag("prod")
	if len(got) != 1 || got[0] != "myapp" {
		t.Errorf("ServicesByTag(prod): want [myapp], got %v", got)
	}

	got = cfg.ServicesByTag("nope")
	if len(got) != 0 {
		t.Errorf("ServicesByTag(nope): want [], got %v", got)
	}
}

func TestAllHostNames(t *testing.T) {
	cfg, err := LoadFrom(writeConfig(t, validYAML))
	if err != nil {
		t.Fatal(err)
	}
	names := cfg.AllHostNames()
	sort.Strings(names)
	if len(names) != 2 {
		t.Fatalf("AllHostNames: want 2, got %d: %v", len(names), names)
	}
	if names[0] != "db1" || names[1] != "web1" {
		t.Errorf("AllHostNames: want [db1 web1], got %v", names)
	}
}

func TestAllServiceNames(t *testing.T) {
	cfg, err := LoadFrom(writeConfig(t, validYAML))
	if err != nil {
		t.Fatal(err)
	}
	names := cfg.AllServiceNames()
	if len(names) != 1 || names[0] != "myapp" {
		t.Errorf("AllServiceNames: want [myapp], got %v", names)
	}
}

func TestParseValidStacks(t *testing.T) {
	yaml := `version: 1
hosts:
  prod:
    address: 1.2.3.4
stacks:
  monitoring:
    host: prod
    path: /home/user/monitoring
    tags: [infra]
`
	cfg, err := LoadFrom(writeConfig(t, yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Stacks) != 1 {
		t.Errorf("stacks: want 1, got %d", len(cfg.Stacks))
	}
	if cfg.Stacks["monitoring"].Path != "/home/user/monitoring" {
		t.Errorf("path: want /home/user/monitoring, got %s", cfg.Stacks["monitoring"].Path)
	}
	names := cfg.AllStackNames()
	if len(names) != 1 || names[0] != "monitoring" {
		t.Errorf("AllStackNames: want [monitoring], got %v", names)
	}
}
