package today_digest_port

import (
	"alt/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

// GetTodayDigestPort reads the today digest projection.
type GetTodayDigestPort interface {
	GetTodayDigest(ctx context.Context, userID uuid.UUID, date time.Time) (domain.TodayDigest, error)
}

// There is deliberately no write port here. today_digest_view is folded from
// the event log by knowledge-sovereign's knowledge_home_projector, which is its
// only writer; alt-backend reads it and nothing more.

// GetProjectionFreshnessPort returns the last updated_at for a projector checkpoint.
type GetProjectionFreshnessPort interface {
	GetProjectionFreshness(ctx context.Context, projectorName string) (*time.Time, error)
}

// CountNeedToKnowItemsPort counts items with pulse_need_to_know why code (page-independent).
type CountNeedToKnowItemsPort interface {
	CountNeedToKnowItems(ctx context.Context, userID uuid.UUID, date time.Time) (int, error)
}
