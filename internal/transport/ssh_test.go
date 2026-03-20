package transport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildHostKeyCallback_Insecure(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "1")

	cb, err := buildHostKeyCallback()
	if err != nil {
		t.Fatalf("expected no error with REMOPS_INSECURE=1, got: %v", err)
	}
	if cb == nil {
		t.Fatal("expected non-nil callback")
	}
}

func TestBuildHostKeyCallback_NoKnownHosts(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "")

	// Override HOME to a temp dir with no .ssh/known_hosts.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	_, err := buildHostKeyCallback()
	if err == nil {
		t.Fatal("expected error when known_hosts does not exist, got nil")
	}
}

func TestBuildHostKeyCallback_ValidKnownHosts(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "")

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("mkdir .ssh: %v", err)
	}

	// Write a minimal valid known_hosts entry (using a real ed25519 key format).
	// This is a test key — not used for any real host.
	knownHostsContent := "example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GkZE\n"
	knownHostsPath := filepath.Join(sshDir, "known_hosts")
	if err := os.WriteFile(knownHostsPath, []byte(knownHostsContent), 0600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	cb, err := buildHostKeyCallback()
	if err != nil {
		t.Fatalf("expected no error with valid known_hosts, got: %v", err)
	}
	if cb == nil {
		t.Fatal("expected non-nil callback")
	}
}
