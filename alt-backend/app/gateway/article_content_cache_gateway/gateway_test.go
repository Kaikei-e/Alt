package article_content_cache_gateway

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

type articleContentDBStub struct {
	loads    int32
	articles map[uuid.UUID]*domain.Article
}

func (s *articleContentDBStub) FetchArticlesByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Article, error) {
	atomic.AddInt32(&s.loads, 1)
	result := make([]*domain.Article, 0, len(ids))
	for _, id := range ids {
		if article, ok := s.articles[id]; ok {
			result = append(result, article)
		}
	}
	return result, nil
}

func TestGateway_GetArticles_PartialMiss(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	db := &articleContentDBStub{
		articles: map[uuid.UUID]*domain.Article{
			id1: {ID: id1, Title: "one", CreatedAt: time.Now()},
			id2: {ID: id2, Title: "two", CreatedAt: time.Now()},
		},
	}
	gateway := newGateway(db)
	gateway.cache.Set(id1, db.articles[id1])

	articles, err := gateway.GetArticles(context.Background(), []uuid.UUID{id1, id2})
	if err != nil {
		t.Fatalf("GetArticles() error = %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("GetArticles() len = %d, want 2", len(articles))
	}
	if atomic.LoadInt32(&db.loads) != 1 {
		t.Fatalf("loads = %d, want 1", db.loads)
	}
}

func TestGateway_InvalidateArticle(t *testing.T) {
	id := uuid.New()
	db := &articleContentDBStub{
		articles: map[uuid.UUID]*domain.Article{
			id: {ID: id, Title: "one", CreatedAt: time.Now()},
		},
	}
	gateway := newGateway(db)

	if _, err := gateway.GetArticles(context.Background(), []uuid.UUID{id}); err != nil {
		t.Fatalf("GetArticles() error = %v", err)
	}
	_ = gateway.InvalidateArticle(context.Background(), id)
	if _, err := gateway.GetArticles(context.Background(), []uuid.UUID{id}); err != nil {
		t.Fatalf("GetArticles() second error = %v", err)
	}
	if atomic.LoadInt32(&db.loads) != 2 {
		t.Fatalf("loads = %d, want 2", db.loads)
	}
}
