package knowledge_lens_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

// CreateLensPort creates a new lens.
type CreateLensPort interface {
	CreateLens(ctx context.Context, lens domain.KnowledgeLens) error
}

// CreateLensVersionPort creates a new lens version.
type CreateLensVersionPort interface {
	CreateLensVersion(ctx context.Context, version domain.KnowledgeLensVersion) error
}

// ListLensesPort lists active (non-archived) lenses for a user.
type ListLensesPort interface {
	ListLenses(ctx context.Context, userID uuid.UUID) ([]domain.KnowledgeLens, error)
}

// GetLensPort gets a single lens by ID.
type GetLensPort interface {
	GetLens(ctx context.Context, lensID uuid.UUID) (*domain.KnowledgeLens, error)
}

// GetCurrentLensVersionPort gets the current lens version for a lens.
type GetCurrentLensVersionPort interface {
	GetCurrentLensVersion(ctx context.Context, lensID uuid.UUID) (*domain.KnowledgeLensVersion, error)
}

// SelectCurrentLensPort sets the active lens for a user.
type SelectCurrentLensPort interface {
	SelectCurrentLens(ctx context.Context, current domain.KnowledgeCurrentLens) error
}

// ArchiveLensPort archives a lens (soft delete).
type ArchiveLensPort interface {
	ArchiveLens(ctx context.Context, lensID uuid.UUID) error
}
