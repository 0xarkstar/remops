package docker

import "github.com/0xarkstar/remops/internal/transport"

// DockerClient wraps a Transport to execute Docker commands on remote hosts.
type DockerClient struct {
	transport transport.Transport
}

// NewDockerClient creates a DockerClient backed by the given transport.
func NewDockerClient(t transport.Transport) *DockerClient {
	return &DockerClient{transport: t}
}
