// Package article_tags_quota_port is the gateway-facing view of the per-user
// tag-generation budget.
package article_tags_quota_port

import (
	"context"
	"errors"
	"time"
)

// ErrBudgetExhausted means the caller has no tag-generation budget right now.
var ErrBudgetExhausted = errors.New("tag generation budget exhausted")

// BudgetExhaustedError is the typed form of ErrBudgetExhausted; RetryAfter is
// the earliest time a retry can succeed.
type BudgetExhaustedError struct {
	RetryAfter time.Duration
}

func (e *BudgetExhaustedError) Error() string { return ErrBudgetExhausted.Error() }

func (e *BudgetExhaustedError) Is(target error) bool { return target == ErrBudgetExhausted }

// ArticleTagsQuotaPort charges the authenticated user for one tag generation.
type ArticleTagsQuotaPort interface {
	// AcquireGeneration takes one token and one in-flight slot for a
	// generation that is about to start. The caller must call release when the
	// generation ends, on every path. A missing user yields an error wrapping
	// domain.ErrInvalidUserContext.
	AcquireGeneration(ctx context.Context) (release func(), err error)
}
