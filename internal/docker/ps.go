package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ContainerInfo holds parsed docker ps output for a single container.
type ContainerInfo struct {
	Host    string `json:"host"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	State   string `json:"state"`
	Health  string `json:"health,omitempty"`
	Ports   string `json:"ports,omitempty"`
	Created string `json:"created,omitempty"`
}

// dockerPSEntry matches the NDJSON output of `docker ps --format '{{json .}}'`.
type dockerPSEntry struct {
	Names     string `json:"Names"`
	Image     string `json:"Image"`
	Status    string `json:"Status"`
	State     string `json:"State"`
	Health    string `json:"Health"`
	Ports     string `json:"Ports"`
	CreatedAt string `json:"CreatedAt"`
}

// ListContainers returns running containers on the named host.
func (d *DockerClient) ListContainers(ctx context.Context, host string) ([]ContainerInfo, error) {
	result, err := d.transport.Exec(ctx, host, "docker ps --format '{{json .}}'")
	if err != nil {
		return nil, fmt.Errorf("docker ps on %s: %w", host, err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("docker ps on %s: %s", host, strings.TrimSpace(result.Stderr))
	}

	var containers []ContainerInfo
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry dockerPSEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // skip malformed lines, return partial results
		}
		containers = append(containers, ContainerInfo{
			Host:    host,
			Name:    entry.Names,
			Image:   entry.Image,
			Status:  entry.Status,
			State:   entry.State,
			Health:  entry.Health,
			Ports:   entry.Ports,
			Created: entry.CreatedAt,
		})
	}
	return containers, nil
}
