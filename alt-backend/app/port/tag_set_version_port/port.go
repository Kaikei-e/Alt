package tag_set_version_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

// CreateTagSetVersionPort creates versioned tag set snapshots.
type CreateTagSetVersionPort interface {
	CreateTagSetVersion(ctx context.Context, tsv domain.TagSetVersion) error
}

// GetTagSetVersionByIDPort reads a specific tag set version by its ID.
type GetTagSetVersionByIDPort interface {
	GetTagSetVersionByID(ctx context.Context, tagSetVersionID uuid.UUID) (domain.TagSetVersion, error)
}
