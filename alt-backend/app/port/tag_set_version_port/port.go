package tag_set_version_port

import (
	"alt/domain"
	"context"
)

// CreateTagSetVersionPort creates versioned tag set snapshots.
type CreateTagSetVersionPort interface {
	CreateTagSetVersion(ctx context.Context, tsv domain.TagSetVersion) error
}
