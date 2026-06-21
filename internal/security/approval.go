package security

import "context"

// Approval is the outcome of an out-of-band approval request.
type Approval struct {
	Approved bool   // whether the operator approved the action
	By       string // approver identity (telegram/discord user id), when known
	Via      string // channel that produced the decision: "telegram" | "discord"
}

// Approver is implemented by anything that can request out-of-band approval.
type Approver interface {
	RequestApproval(ctx context.Context, action string) (Approval, error)
}
