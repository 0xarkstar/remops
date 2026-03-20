package security

import "context"

// Approver is implemented by anything that can request out-of-band approval.
type Approver interface {
	RequestApproval(ctx context.Context, action string) (bool, error)
}
