package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	glssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"
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

// generateEd25519KeyFile writes a PEM-encoded ed25519 private key to a file in dir.
func generateEd25519KeyFile(t *testing.T, dir string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	keyPath := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	return keyPath
}

func TestBuildAuthMethods_NoAgent(t *testing.T) {
	// Unset SSH_AUTH_SOCK so agent path is skipped.
	t.Setenv("SSH_AUTH_SOCK", "")
	// Point HOME at a temp dir with a generated key file.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("mkdir .ssh: %v", err)
	}
	generateEd25519KeyFile(t, sshDir)

	methods, err := buildAuthMethods("")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(methods) == 0 {
		t.Fatal("expected at least one auth method")
	}
}

func TestBuildAuthMethods_ExplicitKey(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	tmpDir := t.TempDir()
	keyPath := generateEd25519KeyFile(t, tmpDir)

	methods, err := buildAuthMethods(keyPath)
	if err != nil {
		t.Fatalf("expected no error with explicit key, got: %v", err)
	}
	if len(methods) == 0 {
		t.Fatal("expected at least one auth method")
	}
}

func TestBuildAuthMethods_NoMethods(t *testing.T) {
	// No agent, no explicit key, no default keys in HOME.
	t.Setenv("SSH_AUTH_SOCK", "")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	// No .ssh directory or key files.

	_, err := buildAuthMethods("")
	if err == nil {
		t.Fatal("expected error when no auth methods available, got nil")
	}
}

func TestSignerFromFile_InvalidKey(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "bad_key")
	if err := os.WriteFile(badPath, []byte("not a valid pem key\n"), 0600); err != nil {
		t.Fatalf("write bad key: %v", err)
	}

	_, err := signerFromFile(badPath)
	if err == nil {
		t.Fatal("expected error for invalid key file, got nil")
	}
}

func TestSignerFromFile_ValidKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := generateEd25519KeyFile(t, tmpDir)

	method, err := signerFromFile(keyPath)
	if err != nil {
		t.Fatalf("expected no error for valid key, got: %v", err)
	}
	if method == nil {
		t.Fatal("expected non-nil auth method")
	}
}

func TestNewSSHTransport(t *testing.T) {
	cfg := &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"test": {Address: "127.0.0.1"},
		},
	}
	tr := NewSSHTransport(cfg)
	if tr == nil {
		t.Fatal("expected non-nil SSHTransport")
	}
	if tr.pool == nil {
		t.Fatal("expected pool to be initialized")
	}
}

func TestClient_UnknownHost(t *testing.T) {
	cfg := &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"known": {Address: "127.0.0.1"},
		},
	}
	tr := NewSSHTransport(cfg)

	_, err := tr.client("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown host, got nil")
	}
}

// startTestSSH starts a minimal mock SSH server and returns its address,
// a path to a client key file, and a cleanup function.
func startTestSSH(t *testing.T) (addr string, keyFile string, cleanup func()) {
	t.Helper()

	_, serverPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverSigner, err := ssh.NewSignerFromKey(serverPriv)
	if err != nil {
		t.Fatalf("server signer: %v", err)
	}

	_, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}

	// Write client key using PKCS8 format (matches integration test pattern).
	keyBytes, err := x509.MarshalPKCS8PrivateKey(clientPriv)
	if err != nil {
		t.Fatalf("marshal client key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}
	f, err := os.CreateTemp(t.TempDir(), "remops-test-key-*")
	if err != nil {
		t.Fatalf("create key temp file: %v", err)
	}
	if err := pem.Encode(f, block); err != nil {
		t.Fatalf("encode key: %v", err)
	}
	f.Close()
	keyFile = f.Name()

	srv := &glssh.Server{
		Handler: func(s glssh.Session) {
			cmd := s.RawCommand()
			switch {
			case strings.Contains(cmd, "echo hello"):
				fmt.Fprint(s, "hello\n")
			case strings.Contains(cmd, "exit 42"):
				s.Exit(42)
			default:
				fmt.Fprintf(s, "cmd: %s\n", cmd)
			}
		},
		PublicKeyHandler: func(_ glssh.Context, _ glssh.PublicKey) bool {
			return true
		},
	}
	srv.AddHostKey(serverSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve(ln) //nolint:errcheck

	return ln.Addr().String(), keyFile, func() {
		srv.Close()
		ln.Close()
	}
}

// makeConfig builds a minimal Config pointing at addr with the given key file.
func makeConfig(t *testing.T, addr, keyFile string) *config.Config {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	port := 0
	fmt.Sscanf(portStr, "%d", &port) //nolint:errcheck
	return &config.Config{
		Version: 1,
		Hosts: map[string]config.Host{
			"test": {
				Address: host,
				Port:    port,
				User:    "testuser",
				Key:     keyFile,
			},
		},
	}
}

func TestSSHTransport_Exec(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "1")
	addr, keyFile, cleanup := startTestSSH(t)
	defer cleanup()

	tr := NewSSHTransport(makeConfig(t, addr, keyFile))
	defer tr.Close()

	result, err := tr.Exec(context.Background(), "test", "echo hello")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("stdout = %q, want %q", result.Stdout, "hello\n")
	}
	if result.Host != "test" {
		t.Errorf("Host = %q, want %q", result.Host, "test")
	}
}

func TestSSHTransport_Exec_ExitCode(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "1")
	addr, keyFile, cleanup := startTestSSH(t)
	defer cleanup()

	tr := NewSSHTransport(makeConfig(t, addr, keyFile))
	defer tr.Close()

	result, err := tr.Exec(context.Background(), "test", "exit 42")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", result.ExitCode)
	}
}

func TestSSHTransport_Stream(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "1")
	addr, keyFile, cleanup := startTestSSH(t)
	defer cleanup()

	tr := NewSSHTransport(makeConfig(t, addr, keyFile))
	defer tr.Close()

	rc, err := tr.Stream(context.Background(), "test", "echo hello")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer rc.Close()

	buf := make([]byte, 64)
	n, _ := rc.Read(buf)
	if !strings.Contains(string(buf[:n]), "hello") {
		t.Errorf("stream output = %q, want to contain %q", string(buf[:n]), "hello")
	}
}

func TestSSHTransport_Ping(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "1")
	addr, keyFile, cleanup := startTestSSH(t)
	defer cleanup()

	tr := NewSSHTransport(makeConfig(t, addr, keyFile))
	defer tr.Close()

	result, err := tr.Ping(context.Background(), "test")
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !result.Online {
		t.Error("expected Online=true")
	}
}

func TestSSHTransport_Close(t *testing.T) {
	cfg := &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"h": {Address: "127.0.0.1"}},
	}
	tr := NewSSHTransport(cfg)
	if err := tr.Close(); err != nil {
		t.Fatalf("Close on empty transport: %v", err)
	}
}

func TestSSHTransport_Exec_UnknownHost(t *testing.T) {
	cfg := &config.Config{
		Version: 1,
		Hosts:   map[string]config.Host{"h": {Address: "127.0.0.1"}},
	}
	tr := NewSSHTransport(cfg)
	defer tr.Close()

	_, err := tr.Exec(context.Background(), "missing", "echo hi")
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
}
