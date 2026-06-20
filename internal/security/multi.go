package security

import (
	"context"
	"fmt"
)

// MultiApprover fans an approval request out to several Approvers concurrently
// and resolves on the first one to return a decision (approve or deny). The
// remaining approvers are cancelled so their prompts reflect "already handled".
//
// This implements the first-responder-wins policy: whichever channel the
// operator acts on first decides the outcome.
type MultiApprover struct {
	approvers []Approver
}

// NewMultiApprover builds a MultiApprover over the given approvers.
func NewMultiApprover(approvers ...Approver) *MultiApprover {
	return &MultiApprover{approvers: approvers}
}

type approvalResult struct {
	approved bool
	err      error
}

// RequestApproval blocks until the first approver returns a decision, ctx is
// cancelled, or every approver fails. A decision from any single channel
// (approve or deny) wins and cancels the others.
func (m *MultiApprover) RequestApproval(ctx context.Context, action string) (bool, error) {
	if len(m.approvers) == 0 {
		return false, fmt.Errorf("multi approver: no approvers configured")
	}

	derived, cancel := context.WithCancel(ctx)
	defer cancel()

	// Buffered to len(approvers) so cancelled goroutines never block on send.
	ch := make(chan approvalResult, len(m.approvers))
	for _, a := range m.approvers {
		a := a
		go func() {
			approved, err := a.RequestApproval(derived, action)
			ch <- approvalResult{approved: approved, err: err}
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
		// First channel to return a clean decision wins; cancel the rest.
		cancel()
		return r.approved, nil
	}

	return false, fmt.Errorf("all approval channels failed: %w", lastErr)
}
