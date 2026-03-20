//go:build integration

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
	"strings"
	"sync/atomic"
	"testing"

	"github.com/0xarkstar/remops/internal/config"
	glssh "github.com/gliderlabs/ssh"
	"golang.org/x/crypto/ssh"
)

// countingListener wraps a net.Listener and counts accepted connections.
type countingListener struct {
	net.Listener
	count atomic.Int32
}

func (l *countingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		l.count.Add(1)
	}
	return conn, err
}

// startMockSSH starts a mock SSH server that accepts any public key.
// Returns the address, a temp file path for the client private key, and cleanup func.
func startMockSSH(t *testing.T) (addr string, keyFile string, cleanup func()) {
	t.Helper()

	// Generate server host key.
	_, serverPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverSigner, err := ssh.NewSignerFromKey(serverPriv)
	if err != nil {
		t.Fatalf("server signer: %v", err)
	}

	// Generate client key and write to temp file.
	_, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	keyFile = writeKeyFile(t, clientPriv)

	srv := &glssh.Server{
		Handler: func(s glssh.Session) {
			cmd := s.RawCommand()
			switch {
			case strings.Contains(cmd, "echo hello"):
				fmt.Fprint(s, "hello\n")
			case strings.Contains(cmd, "exit 42"):
				s.Exit(42)
			case strings.Contains(cmd, "stderr-test"):
				fmt.Fprint(s.Stderr(), "error output\n")
			default:
				fmt.Fprintf(s, "unknown: %s\n", cmd)
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

// writeKeyFile writes an ed25519 private key to a temp file and returns its path.
func writeKeyFile(t *testing.T, priv ed25519.PrivateKey) string {
	t.Helper()
	keyBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}
	f, err := os.CreateTemp("", "remops-test-key-*")
	if err != nil {
		t.Fatalf("create temp key file: %v", err)
	}
	if err := pem.Encode(f, block); err != nil {
		t.Fatalf("encode key: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

// makeTestConfig builds a minimal Config pointing at addr with the given key file.
func makeTestConfig(t *testing.T, addr, keyFile string) *config.Config {
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

func TestSSHExec_BasicCommand(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "1")
	addr, keyFile, cleanup := startMockSSH(t)
	defer cleanup()

	tr := NewSSHTransport(makeTestConfig(t, addr, keyFile))
	defer tr.Close()

	result, err := tr.Exec(context.Background(), "test", "echo hello")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("stdout = %q, want %q", result.Stdout, "hello\n")
	}
}

func TestSSHExec_ExitCode(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "1")
	addr, keyFile, cleanup := startMockSSH(t)
	defer cleanup()

	tr := NewSSHTransport(makeTestConfig(t, addr, keyFile))
	defer tr.Close()

	result, err := tr.Exec(context.Background(), "test", "exit 42")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", result.ExitCode)
	}
}

func TestSSHExec_Stderr(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "1")
	addr, keyFile, cleanup := startMockSSH(t)
	defer cleanup()

	tr := NewSSHTransport(makeTestConfig(t, addr, keyFile))
	defer tr.Close()

	result, err := tr.Exec(context.Background(), "test", "stderr-test")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.Stderr == "" {
		t.Error("expected non-empty stderr")
	}
}

func TestSSHPing(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "1")
	addr, keyFile, cleanup := startMockSSH(t)
	defer cleanup()

	tr := NewSSHTransport(makeTestConfig(t, addr, keyFile))
	defer tr.Close()

	result, err := tr.Ping(context.Background(), "test")
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !result.Online {
		t.Error("expected Online=true")
	}
}

func TestSSHPool_Reuse(t *testing.T) {
	t.Setenv("REMOPS_INSECURE", "1")

	// Generate server host key.
	_, serverPriv, _ := ed25519.GenerateKey(rand.Reader)
	serverSigner, _ := ssh.NewSignerFromKey(serverPriv)

	srv := &glssh.Server{
		Handler: func(s glssh.Session) {
			if strings.Contains(s.RawCommand(), "echo hello") {
				fmt.Fprint(s, "hello\n")
			}
		},
		PublicKeyHandler: func(_ glssh.Context, _ glssh.PublicKey) bool {
			return true
		},
	}
	srv.AddHostKey(serverSigner)

	rawLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	cl := &countingListener{Listener: rawLn}
	go srv.Serve(cl) //nolint:errcheck
	defer srv.Close()
	defer rawLn.Close()

	// Generate and write client key.
	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	keyFile := writeKeyFile(t, clientPriv)

	tr := NewSSHTransport(makeTestConfig(t, rawLn.Addr().String(), keyFile))
	defer tr.Close()

	ctx := context.Background()
	if _, err := tr.Exec(ctx, "test", "echo hello"); err != nil {
		t.Fatalf("first Exec: %v", err)
	}
	if _, err := tr.Exec(ctx, "test", "echo hello"); err != nil {
		t.Fatalf("second Exec: %v", err)
	}

	if n := cl.count.Load(); n != 1 {
		t.Errorf("expected 1 TCP connection (pool reuse), got %d", n)
	}
}
