package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/0xarkstar/remops/internal/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SSHTransport implements Transport over SSH with connection pooling.
type SSHTransport struct {
	cfg  *config.Config
	pool *Pool
}

// NewSSHTransport creates an SSH transport backed by the given config.
func NewSSHTransport(cfg *config.Config) *SSHTransport {
	return &SSHTransport{
		cfg:  cfg,
		pool: NewPool(),
	}
}

// sshPathPrefix is prepended to commands to ensure common binary locations are in PATH.
// Non-interactive SSH sessions often have a minimal PATH that excludes /usr/local/bin.
const sshPathPrefix = "export PATH=/usr/local/bin:/opt/homebrew/bin:$PATH && "

// Exec runs cmd on the named host and returns the result.
func (s *SSHTransport) Exec(ctx context.Context, hostName string, cmd string) (ExecResult, error) {
	cmd = sshPathPrefix + cmd
	start := time.Now()

	client, err := s.client(hostName)
	if err != nil {
		return ExecResult{}, err
	}

	sess, err := client.NewSession()
	if err != nil {
		return ExecResult{}, fmt.Errorf("ssh new session on %s: %w", hostName, err)
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	// Propagate context cancellation by closing the session.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			sess.Close()
		case <-done:
		}
	}()

	err = sess.Run(cmd)
	close(done)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
			err = nil
		} else if ctx.Err() != nil {
			return ExecResult{}, fmt.Errorf("exec cancelled on %s: %w", hostName, ctx.Err())
		} else {
			return ExecResult{}, fmt.Errorf("exec on %s: %w", hostName, err)
		}
	}

	return ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: time.Since(start),
		Host:     hostName,
	}, nil
}

// Stream runs cmd on the named host and returns a reader over its combined output.
func (s *SSHTransport) Stream(ctx context.Context, hostName string, cmd string) (io.ReadCloser, error) {
	cmd = sshPathPrefix + cmd
	client, err := s.client(hostName)
	if err != nil {
		return nil, err
	}

	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh new session on %s: %w", hostName, err)
	}

	pr, pw := io.Pipe()
	sess.Stdout = pw
	sess.Stderr = pw

	if err := sess.Start(cmd); err != nil {
		sess.Close()
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("ssh stream start on %s: %w", hostName, err)
	}

	go func() {
		defer sess.Close()
		// Close pw when context is cancelled or command exits.
		waitDone := make(chan error, 1)
		go func() { waitDone <- sess.Wait() }()
		select {
		case err := <-waitDone:
			pw.CloseWithError(err)
		case <-ctx.Done():
			sess.Close()
			pw.CloseWithError(ctx.Err())
		}
	}()

	return pr, nil
}

// Ping checks connectivity to the named host.
func (s *SSHTransport) Ping(ctx context.Context, hostName string) (PingResult, error) {
	start := time.Now()
	client, err := s.client(hostName)
	if err != nil {
		return PingResult{Host: hostName, Online: false}, nil //nolint:nilerr
	}
	_, _, _ = client.SendRequest("keepalive@openssh.com", true, nil)
	return PingResult{
		Host:    hostName,
		Latency: time.Since(start),
		Online:  true,
	}, nil
}

// Close releases all pooled connections.
func (s *SSHTransport) Close() error {
	return s.pool.Close()
}

// client returns (or creates) a pooled *ssh.Client for the named host.
func (s *SSHTransport) client(hostName string) (*ssh.Client, error) {
	host, ok := s.cfg.Hosts[hostName]
	if !ok {
		return nil, fmt.Errorf("unknown host %q", hostName)
	}

	key := fmt.Sprintf("%s@%s:%d", host.EffectiveUser(), host.Address, host.EffectivePort())
	return s.pool.Get(key, func() (*ssh.Client, error) {
		return s.dial(hostName, host)
	})
}

// dial establishes a new SSH connection to the host, with optional ProxyJump.
func (s *SSHTransport) dial(hostName string, host config.Host) (*ssh.Client, error) {
	authMethods, err := buildAuthMethods(host.Key)
	if err != nil {
		return nil, fmt.Errorf("ssh auth for %s: %w", hostName, err)
	}

	clientCfg := &ssh.ClientConfig{
		User:            host.EffectiveUser(),
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec — TODO: known-hosts support
		Timeout:         host.EffectiveTimeout(),
	}

	addr := fmt.Sprintf("%s:%d", host.Address, host.EffectivePort())

	if host.ProxyJump == "" {
		return ssh.Dial("tcp", addr, clientCfg)
	}

	// ProxyJump: connect to the hop first, then tunnel through.
	hop, ok := s.cfg.Hosts[host.ProxyJump]
	if !ok {
		return nil, fmt.Errorf("proxy_jump host %q not found", host.ProxyJump)
	}

	hopAuthMethods, err := buildAuthMethods(hop.Key)
	if err != nil {
		return nil, fmt.Errorf("ssh auth for proxy %s: %w", host.ProxyJump, err)
	}

	hopCfg := &ssh.ClientConfig{
		User:            hop.EffectiveUser(),
		Auth:            hopAuthMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         hop.EffectiveTimeout(),
	}

	hopAddr := fmt.Sprintf("%s:%d", hop.Address, hop.EffectivePort())
	hopClient, err := ssh.Dial("tcp", hopAddr, hopCfg)
	if err != nil {
		return nil, fmt.Errorf("proxy_jump dial %s: %w", hopAddr, err)
	}

	conn, err := hopClient.Dial("tcp", addr)
	if err != nil {
		hopClient.Close()
		return nil, fmt.Errorf("proxy_jump tunnel to %s: %w", addr, err)
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, clientCfg)
	if err != nil {
		conn.Close()
		hopClient.Close()
		return nil, fmt.Errorf("ssh handshake via proxy to %s: %w", addr, err)
	}

	return ssh.NewClient(ncc, chans, reqs), nil
}

// buildAuthMethods returns auth methods in priority order:
// ssh-agent (via SSH_AUTH_SOCK) → explicit key file → default key files.
func buildAuthMethods(keyFile string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// 1. ssh-agent
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}

	// 2. Explicit key from config
	if keyFile != "" {
		m, err := signerFromFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("load key %s: %w", keyFile, err)
		}
		methods = append(methods, m)
		return methods, nil
	}

	// 3. Default key files
	home, err := os.UserHomeDir()
	if err == nil {
		for _, name := range []string{"id_ed25519", "id_rsa"} {
			path := filepath.Join(home, ".ssh", name)
			if m, err := signerFromFile(path); err == nil {
				methods = append(methods, m)
			}
		}
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH authentication methods available")
	}
	return methods, nil
}

// signerFromFile loads a private key from path and returns an AuthMethod.
func signerFromFile(path string) (ssh.AuthMethod, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("parse private key %s: %w", path, err)
	}
	return ssh.PublicKeys(signer), nil
}
