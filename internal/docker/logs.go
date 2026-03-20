package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// LogOptions configures docker logs behavior.
type LogOptions struct {
	Tail  string // number of lines or "all"
	Since string // timestamp or relative duration
}

// Logs returns the logs for a container on the named host.
func (d *DockerClient) Logs(ctx context.Context, host, container string, opts LogOptions) (string, error) {
	cmd := buildLogsCmd(container, opts)
	result, err := d.transport.Exec(ctx, host, cmd)
	if err != nil {
		return "", fmt.Errorf("docker logs on %s: %w", host, err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("docker logs on %s: %s", host, strings.TrimSpace(result.Stderr))
	}
	return result.Stdout, nil
}

// StreamLogs returns a streaming reader of container logs on the named host.
func (d *DockerClient) StreamLogs(ctx context.Context, host, container string, opts LogOptions) (io.ReadCloser, error) {
	cmd := buildLogsCmd(container, opts) + " --follow"
	rc, err := d.transport.Stream(ctx, host, cmd)
	if err != nil {
		return nil, fmt.Errorf("docker logs stream on %s: %w", host, err)
	}
	return rc, nil
}

func buildLogsCmd(container string, opts LogOptions) string {
	var sb strings.Builder
	sb.WriteString("docker logs ")
	sb.WriteString(container)
	if opts.Tail != "" {
		sb.WriteString(" --tail ")
		sb.WriteString(opts.Tail)
	}
	if opts.Since != "" {
		sb.WriteString(" --since ")
		sb.WriteString(opts.Since)
	}
	return sb.String()
}
