package security

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeApprover is a controllable Approver for testing MultiApprover.
type fakeApprover struct {
	approved  bool
	by        string
	via       string
	err       error
	delay     time.Duration
	cancelled chan struct{} // closed if ctx is cancelled before delay elapses
	cause     error         // cancellation cause observed by a cancelled call
}

func (f *fakeApprover) RequestApproval(ctx context.Context, _ string) (Approval, error) {
	if f.delay == 0 {
		return Approval{Approved: f.approved, By: f.by, Via: f.via}, f.err
	}
	select {
	case <-time.After(f.delay):
		return Approval{Approved: f.approved, By: f.by, Via: f.via}, f.err
	case <-ctx.Done():
		if f.cancelled != nil {
			f.cause = context.Cause(ctx) // synchronized with the test via close below
			close(f.cancelled)
		}
		return Approval{}, ctx.Err()
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
	decision, err := m.RequestApproval(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if !decision.Approved {
		t.Error("expected approved=true from the fast channel")
	}
	waitClosed(t, slow.cancelled, "slow approver should have been cancelled after the fast one decided")
}

func TestMultiApproverDenyWins(t *testing.T) {
	// First responder denies; its decision must win even though another would approve.
	denier := &fakeApprover{approved: false, delay: 5 * time.Millisecond}
	approver := &fakeApprover{approved: true, delay: time.Hour, cancelled: make(chan struct{})}

	m := NewMultiApprover(denier, approver)
	decision, err := m.RequestApproval(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if decision.Approved {
		t.Error("expected approved=false: first responder denied")
	}
}

func TestMultiApproverErrorThenSuccess(t *testing.T) {
	// One channel errors immediately; the decision must come from the healthy one.
	broken := &fakeApprover{err: errors.New("telegram down")}
	healthy := &fakeApprover{approved: true, delay: 5 * time.Millisecond}

	m := NewMultiApprover(broken, healthy)
	decision, err := m.RequestApproval(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if !decision.Approved {
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

func TestMultiApproverPropagatesWinnerAndCancelCause(t *testing.T) {
	winner := &fakeApprover{approved: true, by: "op-123", via: "telegram", delay: 3 * time.Millisecond}
	loser := &fakeApprover{approved: false, via: "discord", delay: time.Hour, cancelled: make(chan struct{})}

	m := NewMultiApprover(winner, loser)
	decision, err := m.RequestApproval(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	// #2: the winning channel's identity propagates out.
	if decision.By != "op-123" || decision.Via != "telegram" {
		t.Errorf("want By=op-123 Via=telegram, got By=%q Via=%q", decision.By, decision.Via)
	}
	// #1: the loser is cancelled with the winning decision as the cause.
	waitClosed(t, loser.cancelled, "loser should have been cancelled")
	var he *handledElsewhereError
	if !errors.As(loser.cause, &he) {
		t.Fatalf("loser cause should be *handledElsewhereError, got %v", loser.cause)
	}
	if !he.winner.Approved || he.winner.Via != "telegram" {
		t.Errorf("cause should carry the winning decision, got %+v", he.winner)
	}
}

func TestCancellationMessage(t *testing.T) {
	// Handled (approved) by another channel.
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(&handledElsewhereError{winner: Approval{Approved: true, Via: "telegram"}})
	<-ctx.Done()
	if msg := cancellationMessage(ctx, "do thing"); !strings.Contains(msg, "Approved on telegram") {
		t.Errorf("want 'Approved on telegram', got %q", msg)
	}

	// Denied by another channel.
	ctx2, cancel2 := context.WithCancelCause(context.Background())
	cancel2(&handledElsewhereError{winner: Approval{Approved: false, Via: "discord"}})
	<-ctx2.Done()
	if msg := cancellationMessage(ctx2, "x"); !strings.Contains(msg, "Denied on discord") {
		t.Errorf("want 'Denied on discord', got %q", msg)
	}

	// Plain timeout (no handledElsewhere cause).
	ctx3, cancel3 := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel3()
	<-ctx3.Done()
	if msg := cancellationMessage(ctx3, "x"); !strings.Contains(msg, "Timed out") {
		t.Errorf("want 'Timed out', got %q", msg)
	}
}
