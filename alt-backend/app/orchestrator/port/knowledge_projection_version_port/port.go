package knowledge_projection_version_port

import (
	"alt/domain"
	"context"
)

// GetActiveVersionPort returns the currently active projection version.
type GetActiveVersionPort interface {
	GetActiveVersion(ctx context.Context) (*domain.KnowledgeProjectionVersion, error)
}

// ListVersionsPort lists all projection versions.
type ListVersionsPort interface {
	ListVersions(ctx context.Context) ([]domain.KnowledgeProjectionVersion, error)
}

// CreateVersionPort creates a new projection version.
type CreateVersionPort interface {
	CreateVersion(ctx context.Context, version domain.KnowledgeProjectionVersion) error
}

// ActivateVersionPort activates a projection version.
type ActivateVersionPort interface {
	ActivateVersion(ctx context.Context, version int) error
}
