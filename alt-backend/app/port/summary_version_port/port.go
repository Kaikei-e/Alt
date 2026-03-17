package summary_version_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

// CreateSummaryVersionPort creates versioned summary artifacts.
type CreateSummaryVersionPort interface {
	CreateSummaryVersion(ctx context.Context, sv domain.SummaryVersion) error
}

// GetLatestSummaryVersionPort gets the latest non-superseded summary.
type GetLatestSummaryVersionPort interface {
	GetLatestSummaryVersion(ctx context.Context, articleID uuid.UUID) (domain.SummaryVersion, error)
}
