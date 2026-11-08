package article_content_cache_gateway

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"alt/domain"
	"alt/utils/cache"

	"github.com/google/uuid"
)

type articleContentDBStub struct {
	loads    int32
	articles map[uuid.UUID]*domain.Article
	onFetch  func(call int32)
}

func (s *articleContentDBStub) FetchArticlesByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Article, error) {
	call := atomic.AddInt32(&s.loads, 1)
	if s.onFetch != nil {
		s.onFetch(call)
	}
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

func TestGateway_GetArticles_ConcurrentStaleHits_DeduplicatesRefresh(t *testing.T) {
	id := uuid.New()
	db := &articleContentDBStub{
		articles: map[uuid.UUID]*domain.Article{
			id: {ID: id, Title: "refreshed", CreatedAt: time.Now()},
		},
	}
	gateway := newGateway(db)
	gateway.cache = cache.NewSharedCache(10*time.Millisecond, time.Second, gateway.loadSingleArticle)
	gateway.cache.Set(id, &domain.Article{ID: id, Title: "initial", CreatedAt: time.Now()})

	// Wait until the entry transitions from fresh to stale.
	time.Sleep(25 * time.Millisecond)

	// While the first background refresh is in flight (before Set updates the
	// cache entry), concurrent stale hits for the same article arrive.
	db.onFetch = func(call int32) {
		if call == 1 {
			for range 8 {
				_, _ = gateway.GetArticles(context.Background(), []uuid.UUID{id})
			}
		}
	}

	got, err := gateway.GetArticles(context.Background(), []uuid.UUID{id})
	if err != nil {
		t.Fatalf("GetArticles() error = %v", err)
	}
	if len(got) != 1 || got[0].Title != "initial" {
		t.Fatalf("stale GetArticles() = %+v, want initial", got)
	}

	deadline := time.Now().Add(time.Second)
	for {
		article, state := gateway.cache.Peek(id)
		if state == cache.CacheStateFresh && article != nil && article.Title == "refreshed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for stale entry to refresh; last state = %v", state)
		}
		time.Sleep(time.Millisecond)
	}

	// Allow any duplicate refresh goroutines spawned during the stale window to settle.
	time.Sleep(20 * time.Millisecond)

	if loads := atomic.LoadInt32(&db.loads); loads != 1 {
		t.Fatalf("stale refresh loader calls = %d, want 1", loads)
	}
}
