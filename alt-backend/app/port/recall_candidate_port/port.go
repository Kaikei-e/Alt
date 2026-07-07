package recall_candidate_port

import (
	"alt/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

// GetRecallCandidatesPort reads recall candidates for a user, ordered by score DESC.
type GetRecallCandidatesPort interface {
	GetRecallCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]domain.RecallCandidate, error)
}

// UpsertRecallCandidatePort writes or updates a recall candidate.
type UpsertRecallCandidatePort interface {
	UpsertRecallCandidate(ctx context.Context, candidate domain.RecallCandidate) error
}

// SnoozeRecallCandidatePort sets a snooze time on a candidate. occurredAt is
// the wall-clock time at which the caller minted this mutation (the command's
// origination time) and is forwarded to knowledge-sovereign so its
// reproject-safe projection can stamp updated_at deterministically instead of
// using SQL now().
type SnoozeRecallCandidatePort interface {
	SnoozeRecallCandidate(ctx context.Context, userID uuid.UUID, itemKey string, until time.Time, occurredAt time.Time) error
}

// DismissRecallCandidatePort removes a candidate from the view. occurredAt is
// the wall-clock time at which the caller minted this mutation, forwarded to
// knowledge-sovereign for the same reproject-determinism reason as
// SnoozeRecallCandidatePort.
type DismissRecallCandidatePort interface {
	DismissRecallCandidate(ctx context.Context, userID uuid.UUID, itemKey string, occurredAt time.Time) error
}

// ArticleFallbackPort retrieves minimal article info for recall display fallback.
// Used when knowledge_home_items projection is missing (projection lag).
type ArticleFallbackPort interface {
	GetArticleTitleAndLink(ctx context.Context, articleID string) (title, link string, publishedAt *time.Time, err error)
}
