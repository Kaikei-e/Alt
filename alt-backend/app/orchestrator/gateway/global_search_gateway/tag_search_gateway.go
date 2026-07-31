package global_search_gateway

import (
	"alt/domain"
	"context"
	"fmt"
	"log/slog"
)

// tagPrefixSearcher is the alt-data-hub read (capability catalog §2.J W3-J4).
type tagPrefixSearcher interface {
	SearchTagsByPrefix(ctx context.Context, prefix string, limit int) ([]domain.GlobalTagHit, error)
}

// TagSearchGateway implements global_search_port.SearchTagsPort.
type TagSearchGateway struct {
	tags   tagPrefixSearcher
	logger *slog.Logger
}

// NewTagSearchGateway creates a new TagSearchGateway.
func NewTagSearchGateway(tags tagPrefixSearcher) *TagSearchGateway {
	if tags == nil {
		panic("global_search_gateway: a tag searcher is required — a nil one would return an empty tag section for every query, which reads as 'no such tag' (see .claude/rules/di-wiring.md)")
	}
	return &TagSearchGateway{tags: tags, logger: slog.Default()}
}

// SearchTagsByPrefix searches for tags matching a prefix.
func (g *TagSearchGateway) SearchTagsByPrefix(ctx context.Context, prefix string, limit int) (*domain.TagSearchSection, error) {
	hits, err := g.tags.SearchTagsByPrefix(ctx, prefix, limit)
	if err != nil {
		g.logger.ErrorContext(ctx, "failed to search tags by prefix", "error", err, "prefix", prefix)
		return nil, fmt.Errorf("search tags by prefix %q: %w", prefix, err)
	}

	return &domain.TagSearchSection{
		Hits:  hits,
		Total: int64(len(hits)),
	}, nil
}
