package article_content_cache_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/cache"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type articleContentDB interface {
	FetchArticlesByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Article, error)
}

type Gateway struct {
	db    articleContentDB
	cache *cache.SharedCache[uuid.UUID, *domain.Article]
}

func NewGateway(db *alt_db.AltDBRepository) *Gateway {
	g := &Gateway{db: db}
	g.cache = cache.NewSharedCache(10*time.Minute, 5*time.Minute, g.loadSingleArticle)
	return g
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
				go func(id uuid.UUID) {
					_, _ = g.cache.Refresh(context.Background(), id)
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
