package security

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeApprover is a controllable Approver for testing MultiApprover.
type fakeApprover struct {
	approved  bool
	err       error
	delay     time.Duration
	cancelled chan struct{} // closed if ctx is cancelled before delay elapses
}

func (f *fakeApprover) RequestApproval(ctx context.Context, _ string) (bool, error) {
	if f.delay == 0 {
		return f.approved, f.err
	}
	select {
	case <-time.After(f.delay):
		return f.approved, f.err
	case <-ctx.Done():
		if f.cancelled != nil {
			close(f.cancelled)
		}
		return false, ctx.Err()
	}
}

func waitClosed(t *testing.T, ch chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal(msg)
	}
}

func TestMultiApproverEmpty(t *testing.T) {
	m := NewMultiApprover()
	if _, err := m.RequestApproval(context.Background(), "x"); err == nil {
		t.Fatal("expected error with no approvers")
	}
}

func TestMultiApproverFirstWinsAndCancelsRest(t *testing.T) {
	fast := &fakeApprover{approved: true, delay: 5 * time.Millisecond}
	slow := &fakeApprover{approved: false, delay: time.Hour, cancelled: make(chan struct{})}

	m := NewMultiApprover(fast, slow)
	approved, err := m.RequestApproval(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if !approved {
		t.Error("expected approved=true from the fast channel")
	}
	waitClosed(t, slow.cancelled, "slow approver should have been cancelled after the fast one decided")
}

func TestMultiApproverDenyWins(t *testing.T) {
	// First responder denies; its decision must win even though another would approve.
	denier := &fakeApprover{approved: false, delay: 5 * time.Millisecond}
	approver := &fakeApprover{approved: true, delay: time.Hour, cancelled: make(chan struct{})}

	m := NewMultiApprover(denier, approver)
	approved, err := m.RequestApproval(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if approved {
		t.Error("expected approved=false: first responder denied")
	}
}

func TestMultiApproverErrorThenSuccess(t *testing.T) {
	// One channel errors immediately; the decision must come from the healthy one.
	broken := &fakeApprover{err: errors.New("telegram down")}
	healthy := &fakeApprover{approved: true, delay: 5 * time.Millisecond}

	m := NewMultiApprover(broken, healthy)
	approved, err := m.RequestApproval(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if !approved {
		t.Error("expected approved=true from the healthy channel despite the other erroring")
	}
}

func TestMultiApproverAllFail(t *testing.T) {
	a := &fakeApprover{err: errors.New("a down")}
	b := &fakeApprover{err: errors.New("b down")}

	m := NewMultiApprover(a, b)
	if _, err := m.RequestApproval(context.Background(), "deploy"); err == nil {
		t.Fatal("expected error when all channels fail")
	}
}

func TestMultiApproverParentCancel(t *testing.T) {
	a := &fakeApprover{approved: true, delay: time.Hour, cancelled: make(chan struct{})}
	b := &fakeApprover{approved: true, delay: time.Hour, cancelled: make(chan struct{})}

	m := NewMultiApprover(a, b)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before the call

	if _, err := m.RequestApproval(ctx, "deploy"); err == nil {
		t.Fatal("expected error when parent context is cancelled")
	}
}
