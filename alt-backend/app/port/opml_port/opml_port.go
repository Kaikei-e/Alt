package opml_port

import (
	"alt/domain"
	"context"
)

// ExportOPMLPort retrieves feed links with metadata for OPML export.
type ExportOPMLPort interface {
	FetchFeedLinksForExport(ctx context.Context) ([]*domain.FeedLinkForExport, error)
}

// ImportOPMLPort registers feed link URLs in bulk for OPML import.
type ImportOPMLPort interface {
	RegisterFeedLinkBulk(ctx context.Context, urls []string) (*domain.OPMLImportResult, error)
}
