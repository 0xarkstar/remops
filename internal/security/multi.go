package security

import (
	"context"
	"errors"
	"fmt"
)

// handledElsewhereError is the cancellation cause MultiApprover sets when one
// channel produces a decision, so the cancelled channels can display
// "Approved/Denied on <channel>" instead of an ambiguous timeout.
type handledElsewhereError struct{ winner Approval }

func (e *handledElsewhereError) Error() string { return "approval handled by another channel" }

// cancellationMessage renders the final prompt text for an approval that was
// cancelled before the operator acted on THIS channel: either another channel
// handled it (multi) or the request timed out.
func cancellationMessage(ctx context.Context, action string) string {
	var he *handledElsewhereError
	if errors.As(context.Cause(ctx), &he) {
		icon, verb := "✅", "Approved"
		if !he.winner.Approved {
			icon, verb = "❌", "Denied"
		}
		via := he.winner.Via
		if via == "" {
			via = "another channel"
		}
		return fmt.Sprintf("%s %s on %s: %s", icon, verb, via, action)
	}
	return fmt.Sprintf("⏰ Timed out: %s", action)
}

// MultiApprover fans an approval request out to several Approvers concurrently
// and resolves on the first one to return a decision (approve or deny). The
// remaining approvers are cancelled with a cause carrying the winning decision.
type MultiApprover struct {
	approvers []Approver
}

// NewMultiApprover builds a MultiApprover over the given approvers.
func NewMultiApprover(approvers ...Approver) *MultiApprover {
	return &MultiApprover{approvers: approvers}
}

type approvalResult struct {
	approval Approval
	err      error
}

// RequestApproval blocks until the first approver returns a decision, ctx is
// cancelled, or every approver fails. A decision from any single channel wins
// and cancels the others.
func (m *MultiApprover) RequestApproval(ctx context.Context, action string) (Approval, error) {
	if len(m.approvers) == 0 {
		return Approval{}, fmt.Errorf("multi approver: no approvers configured")
	}

	derived, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	// Buffered to len(approvers) so cancelled goroutines never block on send.
	ch := make(chan approvalResult, len(m.approvers))
	for _, a := range m.approvers {
		a := a
		go func() {
			approval, err := a.RequestApproval(derived, action)
			ch <- approvalResult{approval: approval, err: err}
		}()
	}

	var lastErr error
	errCount := 0
	for range m.approvers {
		r := <-ch
		if r.err != nil {
			errCount++
			lastErr = r.err
			continue
		}
		// First clean decision wins; cancel the rest with the winning decision
		// as the cause so they can render "Approved/Denied on <channel>".
		cancel(&handledElsewhereError{winner: r.approval})
		return r.approval, nil
	}

	return Approval{}, fmt.Errorf("all approval channels failed: %w", lastErr)
}
