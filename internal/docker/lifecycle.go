package docker

import (
	"context"
	"fmt"
	"strings"
)

// Restart restarts the named container on the given host.
func (d *DockerClient) Restart(ctx context.Context, host, container string) error {
	return d.runLifecycleCmd(ctx, host, "restart", container)
}

// Stop stops the named container on the given host.
func (d *DockerClient) Stop(ctx context.Context, host, container string) error {
	return d.runLifecycleCmd(ctx, host, "stop", container)
}

// Start starts the named container on the given host.
func (d *DockerClient) Start(ctx context.Context, host, container string) error {
	return d.runLifecycleCmd(ctx, host, "start", container)
}

func (d *DockerClient) runLifecycleCmd(ctx context.Context, host, action, container string) error {
	result, err := d.transport.Exec(ctx, host, fmt.Sprintf("docker %s %s", action, container))
	if err != nil {
		return fmt.Errorf("docker %s %s on %s: %w", action, container, host, err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("docker %s %s on %s: %s", action, container, host, strings.TrimSpace(result.Stderr))
	}
	return nil
}
