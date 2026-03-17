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

// UpsertTodayDigestPort writes to the today digest projection.
type UpsertTodayDigestPort interface {
	UpsertTodayDigest(ctx context.Context, digest domain.TodayDigest) error
}
