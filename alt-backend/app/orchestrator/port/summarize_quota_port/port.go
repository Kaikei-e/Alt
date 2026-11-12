// Package summarize_quota_port is the usecase-facing view of the per-user
// summarization budget.
package summarize_quota_port

import (
	"context"
	"errors"
)

// ErrBudgetExhausted means the caller has no summarization budget left right
// now; the caller should skip generation rather than fail the request.
var ErrBudgetExhausted = errors.New("summarization budget exhausted")

// SummarizeQuotaPort charges the authenticated user's summarization budget.
type SummarizeQuotaPort interface {
	// ChargeSummarization takes one token for a summary that is about to be
	// generated. It returns ErrBudgetExhausted when none is left.
	ChargeSummarization(ctx context.Context) error
}
