package mcp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestServerRun(t *testing.T) {
	s := NewServer(minimalConfig(), nil)

	// Pipe for stdin
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// Pipe for stdout
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	os.Stdin = stdinR
	os.Stdout = stdoutW
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	// Write one request then close stdin to terminate Run
	go func() {
		fmt.Fprintln(stdinW, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
		stdinW.Close()
	}()

	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	stdoutW.Close()

	var buf bytes.Buffer
	io.Copy(&buf, stdoutR)
	stdoutR.Close()

	if !strings.Contains(buf.String(), "remops") {
		t.Errorf("expected 'remops' in response, got: %s", buf.String())
	}
}

func TestServerRunParseError(t *testing.T) {
	s := NewServer(minimalConfig(), nil)

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	os.Stdin = stdinR
	os.Stdout = stdoutW
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	go func() {
		fmt.Fprintln(stdinW, `{invalid json}`)
		stdinW.Close()
	}()

	s.Run(context.Background())
	stdoutW.Close()

	var buf bytes.Buffer
	io.Copy(&buf, stdoutR)
	stdoutR.Close()

	if !strings.Contains(buf.String(), "parse error") {
		t.Errorf("expected parse error in response, got: %s", buf.String())
	}
}

func TestServerRunNotification(t *testing.T) {
	// Notifications have no ID and should not produce a response.
	s := NewServer(minimalConfig(), nil)

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	os.Stdin = stdinR
	os.Stdout = stdoutW
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	go func() {
		// Notification: no "id" field
		fmt.Fprintln(stdinW, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		stdinW.Close()
	}()

	s.Run(context.Background())
	stdoutW.Close()

	var buf bytes.Buffer
	io.Copy(&buf, stdoutR)
	stdoutR.Close()

	// No response should be written for notifications
	if strings.TrimSpace(buf.String()) != "" {
		t.Errorf("expected no response for notification, got: %s", buf.String())
	}
}
