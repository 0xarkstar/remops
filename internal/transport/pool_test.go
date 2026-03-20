package transport

import (
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// trackConn implements ssh.Conn and records Close calls.
type trackConn struct {
	closed bool
}

func (c *trackConn) User() string          { return "" }
func (c *trackConn) SessionID() []byte     { return nil }
func (c *trackConn) ClientVersion() []byte { return nil }
func (c *trackConn) ServerVersion() []byte { return nil }
func (c *trackConn) RemoteAddr() net.Addr  { return &net.TCPAddr{} }
func (c *trackConn) LocalAddr() net.Addr   { return &net.TCPAddr{} }
func (c *trackConn) SendRequest(_ string, _ bool, _ []byte) (bool, []byte, error) {
	return false, nil, errors.New("fake")
}
func (c *trackConn) OpenChannel(_ string, _ []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, errors.New("fake")
}
func (c *trackConn) Close() error { c.closed = true; return nil }
func (c *trackConn) Wait() error  { return nil }

func fakeClient() (*ssh.Client, *trackConn) {
	tc := &trackConn{}
	return &ssh.Client{Conn: tc}, tc
}

func TestPoolClosesHopOnClose(t *testing.T) {
	p := NewPool()

	client, clientTrack := fakeClient()
	hop, hopTrack := fakeClient()

	p.mu.Lock()
	p.entries["proxied"] = &poolEntry{client: client, hop: hop, lastUsed: time.Now()}
	p.mu.Unlock()

	if err := p.Close(); err != nil {
		t.Fatalf("pool.Close: %v", err)
	}

	if !clientTrack.closed {
		t.Error("expected pool.Close() to close the main client")
	}
	if !hopTrack.closed {
		t.Error("expected pool.Close() to close the hop client")
	}
}

func TestPoolClosesHopOnEviction(t *testing.T) {
	p := NewPool()

	client, clientTrack := fakeClient()
	hop, hopTrack := fakeClient()

	// lastUsed zero value is far in the past → stale, will be evicted.
	p.mu.Lock()
	p.entries["proxied"] = &poolEntry{client: client, hop: hop}
	p.mu.Unlock()

	// A Get call with any key triggers the eviction sweep.
	_, _ = p.Get("other", func() (*ssh.Client, *ssh.Client, error) {
		return nil, nil, errors.New("unused")
	})

	if !clientTrack.closed {
		t.Error("expected stale client to be closed during eviction")
	}
	if !hopTrack.closed {
		t.Error("expected stale hop to be closed during eviction")
	}
}

func TestPoolGetDialReturnsHop(t *testing.T) {
	p := NewPool()

	client, _ := fakeClient()
	hop, hopTrack := fakeClient()

	got, err := p.Get("k", func() (*ssh.Client, *ssh.Client, error) {
		return client, hop, nil
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != client {
		t.Error("Get should return the client, not the hop")
	}

	// Verify hop is stored and closed on pool.Close.
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !hopTrack.closed {
		t.Error("hop should be closed when pool closes")
	}
}
