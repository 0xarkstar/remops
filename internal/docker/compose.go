package docker

import (
	"context"
	"fmt"
	"strings"
)

// ComposePS returns the output of `docker compose ps` in the given directory.
func (d *DockerClient) ComposePS(ctx context.Context, host, dir string) (string, error) {
	cmd := fmt.Sprintf("cd %s && docker compose ps --format json", shellQuote(dir))
	result, err := d.transport.Exec(ctx, host, cmd)
	if err != nil {
		return "", fmt.Errorf("compose ps on %s: %w", host, err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("compose ps on %s: %s", host, strings.TrimSpace(result.Stderr))
	}
	return result.Stdout, nil
}

// ComposeAction runs a compose action (up -d, pull, down, restart) in the given directory.
func (d *DockerClient) ComposeAction(ctx context.Context, host, dir, action string) (string, int, error) {
	cmd := fmt.Sprintf("cd %s && docker compose %s", shellQuote(dir), action)
	result, err := d.transport.Exec(ctx, host, cmd)
	if err != nil {
		return "", -1, fmt.Errorf("compose %s on %s: %w", action, host, err)
	}
	combined := result.Stdout
	if result.Stderr != "" {
		combined += result.Stderr
	}
	return combined, result.ExitCode, nil
}

// ComposeLogs returns logs for a compose stack, optionally filtered by service.
func (d *DockerClient) ComposeLogs(ctx context.Context, host, dir string, tail int, since, serviceName string) (string, error) {
	cmd := fmt.Sprintf("cd %s && docker compose logs", shellQuote(dir))
	if tail > 0 {
		cmd += fmt.Sprintf(" --tail %d", tail)
	}
	if since != "" {
		cmd += " --since " + since
	}
	if serviceName != "" {
		cmd += " " + serviceName
	}
	result, err := d.transport.Exec(ctx, host, cmd)
	if err != nil {
		return "", fmt.Errorf("compose logs on %s: %w", host, err)
	}
	combined := result.Stdout
	if result.Stderr != "" {
		combined += result.Stderr
	}
	return combined, nil
}

// shellQuote wraps a path in single quotes for safe shell use.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
