package transport

import (
	"context"
	"testing"
)

func TestLocalTransportStreamStartError(t *testing.T) {
	lt := NewLocalTransport()
	// Command that doesn't exist — Start() should fail
	_, err := lt.Stream(context.Background(), "localhost", "no_such_command_remops_xyz_123456")
	if err == nil {
		t.Error("expected error for non-existent command, got nil")
	}
}
