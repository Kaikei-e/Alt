package article_content_cache_gateway

import (
	"alt/domain"
	"alt/utils/cache"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// articleContentDB is catalog §2.C W3-C5, served by alt-data-hub since
// ADR-000954 Wave 3 batch 2. The cache in front of it stays here: a TTL is
// flow orchestration, which D4 keeps on the calling side.
type articleContentDB interface {
	FetchArticlesByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Article, error)
}

type Gateway struct {
	db    articleContentDB
	cache *cache.SharedCache[uuid.UUID, *domain.Article]
}

// NewGateway panics on a nil source: the cache would answer every miss with
// a nil dereference on a user request rather than failing at boot.
func NewGateway(db articleContentDB) *Gateway {
	if db == nil {
		panic("article_content_cache_gateway: an article batch reader is required — articles moved to alt-data-hub in ADR-000954 Wave 3")
	}
	return newGateway(db)
}

func newGateway(db articleContentDB) *Gateway {
	g := &Gateway{db: db}
	g.cache = cache.NewSharedCache(10*time.Minute, 5*time.Minute, g.loadSingleArticle)
	return g
}

func (g *Gateway) GetArticles(ctx context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error) {
	if len(articleIDs) == 0 {
		return []*domain.Article{}, nil
	}

	results := make(map[uuid.UUID]*domain.Article, len(articleIDs))
	misses := make([]uuid.UUID, 0)

	for _, id := range articleIDs {
		if article, state := g.cache.Peek(id); state == cache.CacheStateFresh || state == cache.CacheStateStale {
			results[id] = article
			if state == cache.CacheStateStale {
				// Detach cancellation so refresh outlives the request, but keep
				// request-derived values (trace/baggage) via WithoutCancel.
				refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				go func(id uuid.UUID) {
					defer cancel()
					if _, err := g.cache.Refresh(refreshCtx, id); err != nil {
						slog.Default().Error("article content cache background refresh failed",
							"article_id", id,
							"error", err,
						)
					}
				}(id)
			}
			continue
		}
		misses = append(misses, id)
	}

	if len(misses) > 0 {
		articles, err := g.db.FetchArticlesByIDs(ctx, misses)
		if err != nil {
			return nil, fmt.Errorf("fetch articles by ids: %w", err)
		}
		for _, article := range articles {
			results[article.ID] = article
			g.cache.Set(article.ID, article)
		}
	}

	ordered := make([]*domain.Article, 0, len(articleIDs))
	for _, id := range articleIDs {
		if article, ok := results[id]; ok {
			ordered = append(ordered, article)
		}
	}
	return ordered, nil
}

func (g *Gateway) InvalidateArticle(ctx context.Context, articleID uuid.UUID) error {
	g.cache.Invalidate(articleID)
	return nil
}

func (g *Gateway) loadSingleArticle(ctx context.Context, articleID uuid.UUID) (*domain.Article, error) {
	articles, err := g.db.FetchArticlesByIDs(ctx, []uuid.UUID{articleID})
	if err != nil {
		return nil, fmt.Errorf("fetch article by id: %w", err)
	}
	if len(articles) == 0 {
		return nil, nil
	}
	return articles[0], nil
}
