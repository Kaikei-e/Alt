package global_search_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"errors"
	"log/slog"
)

// TagSearchGateway implements global_search_port.SearchTagsPort.
type TagSearchGateway struct {
	tagRepo *alt_db.TagRepository
	logger  *slog.Logger
}

// NewTagSearchGateway creates a new TagSearchGateway.
func NewTagSearchGateway(tagRepo *alt_db.TagRepository) *TagSearchGateway {
	return &TagSearchGateway{
		tagRepo: tagRepo,
		logger:  slog.Default(),
	}
}

// SearchTagsByPrefix searches for tags matching a prefix.
func (g *TagSearchGateway) SearchTagsByPrefix(ctx context.Context, prefix string, limit int) (*domain.TagSearchSection, error) {
	if g.tagRepo == nil {
		return nil, errors.New("tag repository not available")
	}

	hits, err := g.tagRepo.SearchTagsByPrefix(ctx, prefix, limit)
	if err != nil {
		g.logger.ErrorContext(ctx, "failed to search tags by prefix", "error", err, "prefix", prefix)
		return nil, err
	}

	return &domain.TagSearchSection{
		Hits:  hits,
		Total: int64(len(hits)),
	}, nil
}
