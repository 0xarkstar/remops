package cmd

import (
	"testing"

	"github.com/0xarkstar/remops/internal/config"
)

func TestEscapeSingleQuotes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"SELECT 1", "SELECT 1"},
		{"SELECT * FROM users WHERE name = 'alice'", "SELECT * FROM users WHERE name = '\\''alice'\\''"},
		{"it's a test", "it'\\''s a test"},
		{"no quotes here", "no quotes here"},
		{"", ""},
	}
	for _, tt := range tests {
		got := escapeSingleQuotes(tt.input)
		if got != tt.want {
			t.Errorf("escapeSingleQuotes(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildDBExecCmd_PostgreSQL(t *testing.T) {
	svc := config.Service{
		Host:      "prod",
		Container: "myapp_db",
		DB: &config.DBConfig{
			Engine:   "postgresql",
			User:     "admin",
			Database: "mydb",
		},
	}
	cmd, err := buildDBExecCmd(svc, "SELECT 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "docker exec myapp_db psql -U admin -d mydb -c 'SELECT 1'"
	if cmd != want {
		t.Errorf("got %q, want %q", cmd, want)
	}
}

func TestBuildDBExecCmd_MySQL(t *testing.T) {
	svc := config.Service{
		Host:      "prod",
		Container: "mysql_db",
		DB: &config.DBConfig{
			Engine:   "mysql",
			User:     "root",
			Password: "secret",
			Database: "app",
		},
	}
	cmd, err := buildDBExecCmd(svc, "SHOW TABLES")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "docker exec mysql_db mysql -u root -psecret app -e 'SHOW TABLES'"
	if cmd != want {
		t.Errorf("got %q, want %q", cmd, want)
	}
}

func TestBuildDBExecCmd_UnsupportedEngine(t *testing.T) {
	svc := config.Service{
		Host:      "prod",
		Container: "mongo_db",
		DB: &config.DBConfig{
			Engine:   "mongodb",
			User:     "admin",
			Database: "test",
		},
	}
	_, err := buildDBExecCmd(svc, "db.test.find()")
	if err == nil {
		t.Fatal("expected error for unsupported engine, got nil")
	}
}

func TestBuildDBExecCmd_SingleQuoteEscape(t *testing.T) {
	svc := config.Service{
		Host:      "prod",
		Container: "pg_db",
		DB: &config.DBConfig{
			Engine:   "postgresql",
			User:     "admin",
			Database: "mydb",
		},
	}
	sql := "SELECT * FROM users WHERE name = 'alice'"
	cmd, err := buildDBExecCmd(svc, sql)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "docker exec pg_db psql -U admin -d mydb -c 'SELECT * FROM users WHERE name = '\\''alice'\\'''"
	if cmd != want {
		t.Errorf("got %q, want %q", cmd, want)
	}
}

func TestIsWriteQuery(t *testing.T) {
	tests := []struct {
		sql   string
		write bool
	}{
		{"SELECT * FROM users", false},
		{"select id from t", false},
		{"SHOW TABLES", false},
		{"EXPLAIN SELECT 1", false},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", false},
		{"INSERT INTO t VALUES (1)", true},
		{"UPDATE t SET x=1", true},
		{"DELETE FROM t", true},
		{"DROP TABLE t", true},
	}
	for _, tt := range tests {
		got := isWriteQuery(tt.sql)
		if got != tt.write {
			t.Errorf("isWriteQuery(%q) = %v, want %v", tt.sql, got, tt.write)
		}
	}
}

func TestConfigDBParsing(t *testing.T) {
	svc := config.Service{
		Host:      "prod",
		Container: "app_db",
		DB: &config.DBConfig{
			Engine:   "postgresql",
			User:     "pguser",
			Password: "pgpass",
			Database: "appdb",
			Port:     5432,
		},
	}

	if svc.DB == nil {
		t.Fatal("expected DB config to be set")
	}
	if svc.DB.Engine != "postgresql" {
		t.Errorf("expected engine postgresql, got %s", svc.DB.Engine)
	}
	if svc.DB.Port != 5432 {
		t.Errorf("expected port 5432, got %d", svc.DB.Port)
	}
}
