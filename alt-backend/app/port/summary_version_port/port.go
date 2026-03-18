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

// GetSummaryVersionByIDPort gets a specific summary version by its ID.
// Used by the projector to ensure reproject-safe resolution (not latest).
type GetSummaryVersionByIDPort interface {
	GetSummaryVersionByID(ctx context.Context, summaryVersionID uuid.UUID) (domain.SummaryVersion, error)
}

// MarkSummaryVersionSupersededPort marks all non-superseded versions as superseded by the new version.
// Returns the previous latest version (before marking), or nil if none existed.
type MarkSummaryVersionSupersededPort interface {
	MarkSummaryVersionSuperseded(ctx context.Context, articleID uuid.UUID, newVersionID uuid.UUID) (*domain.SummaryVersion, error)
}
