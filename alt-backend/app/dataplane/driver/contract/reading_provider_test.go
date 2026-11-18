//go:build contract

package contract

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"alt/domain"
	"alt/gen/proto/alt/reading/v1/readingv1connect"
	"alt/orchestrator/connect/v2/reading"
	"alt/orchestrator/usecase/reading_queue_usecase"
	"alt/orchestrator/usecase/record_reading_events_usecase"
)

type pactReadingState struct {
	mu   sync.Mutex
	mode string // "empty", "full", "active"
	seq  atomic.Int64
}

var readingStubState = &pactReadingState{mode: "empty"}

func (p *pactReadingState) setMode(mode string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode = mode
}

func (p *pactReadingState) getMode() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mode == "" {
		return "empty"
	}
	return p.mode
}

func (p *pactReadingState) AppendKnowledgeEvent(_ context.Context, _ domain.KnowledgeEvent) (int64, error) {
	return p.seq.Add(1), nil
}

func (p *pactReadingState) LookupArticleSource(_ context.Context, _ string, _ uuid.UUID) (domain.ArticleSource, error) {
	return domain.ArticleSource{
		Title: "Queued Article",
		URL:   "https://example.com/articles/1",
	}, nil
}

func (p *pactReadingState) MarkArticleAsRead(_ context.Context, _ url.URL, _ uuid.UUID) error {
	return nil
}

func (p *pactReadingState) GetReadingQueue(_ context.Context, _ uuid.UUID, _ time.Time) ([]domain.ReadingQueueEntry, error) {
	queuedAt := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	primaryID := uuid.MustParse("11111111-1111-4111-8111-111111111111")

	switch p.getMode() {
	case "full":
		entries := make([]domain.ReadingQueueEntry, 0, 7)
		entries = append(entries, domain.ReadingQueueEntry{
			ArticleID:          primaryID,
			QueuedAt:           queuedAt,
			ExpiresAt:          expiresAt,
			QueuePolicyVersion: 1,
		})
		for i := 2; i <= 7; i++ {
			entries = append(entries, domain.ReadingQueueEntry{
				ArticleID:          uuid.MustParse(fmt.Sprintf("%d%d%d%d%d%d%d%d-1111-4111-8111-111111111111", i, i, i, i, i, i, i, i)),
				QueuedAt:           queuedAt,
				ExpiresAt:          expiresAt,
				QueuePolicyVersion: 1,
			})
		}
		return entries, nil
	case "active":
		return []domain.ReadingQueueEntry{
			{
				ArticleID:          primaryID,
				QueuedAt:           queuedAt,
				ExpiresAt:          expiresAt,
				QueuePolicyVersion: 1,
			},
		}, nil
	default:
		return nil, nil
	}
}

func (p *pactReadingState) CountReadingSourcesSince(_ context.Context, _ uuid.UUID, _ time.Time) (int, error) {
	return 2, nil
}

func (p *pactReadingState) FetchArticlesByIDs(_ context.Context, ids []uuid.UUID) ([]*domain.Article, error) {
	articles := make([]*domain.Article, 0, len(ids))
	for _, id := range ids {
		articles = append(articles, &domain.Article{
			ID:    id,
			Title: "Queued Article",
			URL:   "https://example.com/articles/1",
		})
	}
	return articles, nil
}

func (p *pactReadingState) GetOgImageURLsByArticleIDs(_ context.Context, _ []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func mountReadingServiceProcedures(mux *http.ServeMux) {
	fixedNow := func() time.Time { return time.Date(2026, 11, 18, 0, 5, 0, 0, time.UTC) }
	recordUC := record_reading_events_usecase.NewRecordReadingEventsUsecase(
		readingStubState,
		readingStubState,
		readingStubState,
		fixedNow,
	)
	queueUC := reading_queue_usecase.NewQueueArticleUsecase(
		readingStubState,
		readingStubState,
		readingStubState,
		readingStubState,
		readingStubState,
		7,
		48,
		fixedNow,
	)
	unqueueUC := reading_queue_usecase.NewUnqueueArticleUsecase(
		readingStubState,
		fixedNow,
	)
	getQueueUC := reading_queue_usecase.NewGetReadTodayQueueUsecase(
		readingStubState,
		readingStubState,
		readingStubState,
		readingStubState,
		7,
		fixedNow,
	)
	handler := reading.NewHandler(recordUC, queueUC, unqueueUC, getQueueUC, nil, slog.Default())
	path, connectHandler := readingv1connect.NewReadingServiceHandler(handler)
	mux.Handle(path, withPactUserContext(connectHandler))
}
