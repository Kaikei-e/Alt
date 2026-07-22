package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"search-indexer/domain"
	"search-indexer/port"
	"search-indexer/usecase"
	"sync"
	"testing"
	"time"

	"github.com/ikawaha/kagome/v2/tokenizer"
)

// mockArticleRepo implements port.ArticleRepository for testing.
type mockArticleRepo struct {
	articles map[string]*domain.Article
	err      error
}

func (m *mockArticleRepo) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	return nil, nil, "", m.err
}

func (m *mockArticleRepo) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	return nil, nil, "", m.err
}

func (m *mockArticleRepo) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]string, *time.Time, error) {
	return nil, nil, m.err
}

func (m *mockArticleRepo) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	return nil, m.err
}

func (m *mockArticleRepo) GetArticleByID(ctx context.Context, articleID string) (*domain.Article, error) {
	if m.err != nil {
		return nil, m.err
	}
	if a, ok := m.articles[articleID]; ok {
		return a, nil
	}
	return nil, &domain.RepositoryError{Op: "GetArticleByID", Err: errors.New("not found")}
}

// mockSearchEngine implements port.SearchEngine for testing.
type mockSearchEngine struct {
	indexedDocs []domain.SearchDocument
	err         error
}

func (m *mockSearchEngine) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	if m.err != nil {
		return m.err
	}
	m.indexedDocs = append(m.indexedDocs, docs...)
	return nil
}

func (m *mockSearchEngine) DeleteDocuments(ctx context.Context, ids []string) error { return m.err }
func (m *mockSearchEngine) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	return nil, m.err
}
func (m *mockSearchEngine) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
	return nil, m.err
}
func (m *mockSearchEngine) SearchWithDateFilter(ctx context.Context, query string, publishedAfter, publishedBefore *time.Time, limit int) ([]domain.SearchDocument, error) {
	return nil, m.err
}
func (m *mockSearchEngine) EnsureIndex(ctx context.Context) error { return m.err }
func (m *mockSearchEngine) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error) {
	return nil, m.err
}
func (m *mockSearchEngine) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error) {
	return nil, 0, m.err
}
func (m *mockSearchEngine) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	return m.err
}

func (m *mockSearchEngine) PruneTaskHistory(ctx context.Context, olderThan time.Duration) error {
	return nil
}

var _ port.SearchEngine = (*mockSearchEngine)(nil)
var _ port.ArticleRepository = (*mockArticleRepo)(nil)

func TestIndexEventHandler_HandleEvent_ArticleCreated(t *testing.T) {
	now := time.Now()
	article, _ := domain.NewArticle("art-1", "Test Title", "Test Content", []string{"go"}, now, "user-1")

	repo := &mockArticleRepo{
		articles: map[string]*domain.Article{"art-1": article},
	}
	se := &mockSearchEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	payload, _ := json.Marshal(ArticleCreatedPayload{
		ArticleID: "art-1",
		UserID:    "user-1",
		Title:     "Test Title",
	})

	err := handler.HandleEvent(context.Background(), Event{
		EventType: "ArticleCreated",
		EventID:   "evt-1",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	// Wait for the flush timer (2s) or manual stop
	handler.Stop()

	if len(se.indexedDocs) != 1 {
		t.Errorf("expected 1 indexed doc, got %d", len(se.indexedDocs))
	}
}

// ArticleUpdated shares the fat-event payload shape with ArticleCreated.
// alt-backend publishes it whenever an article's content/tags change; before
// this handler existed the events fell through the default branch and the
// search index silently went stale against provider-side updates.
func TestIndexEventHandler_HandleEvent_ArticleUpdated_FatEvent(t *testing.T) {
	repo := &mockArticleRepo{articles: map[string]*domain.Article{}}
	se := &mockSearchEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	payload, _ := json.Marshal(ArticleCreatedPayload{
		ArticleID: "art-upd-1",
		UserID:    "user-1",
		Title:     "Updated Title",
		Content:   "Updated content body",
		Tags:      []string{"go", "updated"},
	})

	err := handler.HandleEvent(context.Background(), Event{
		EventType: "ArticleUpdated",
		EventID:   "evt-upd-1",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("HandleEvent(ArticleUpdated) error = %v", err)
	}

	// Flush the fat-event buffer.
	handler.Stop()

	if len(se.indexedDocs) != 1 {
		t.Fatalf("expected 1 indexed doc for ArticleUpdated, got %d", len(se.indexedDocs))
	}
	doc := se.indexedDocs[0]
	if doc.ID != "art-upd-1" || doc.Title != "Updated Title" || doc.Content != "Updated content body" {
		t.Errorf("upsert payload wrong: %+v", doc)
	}
}

func TestIndexEventHandler_HandleEvent_IndexArticle(t *testing.T) {
	now := time.Now()
	article, _ := domain.NewArticle("art-2", "Another Title", "Another Content", []string{"rust"}, now, "user-2")

	repo := &mockArticleRepo{
		articles: map[string]*domain.Article{"art-2": article},
	}
	se := &mockSearchEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	payload, _ := json.Marshal(IndexArticlePayload{
		ArticleID: "art-2",
		UserID:    "user-2",
	})

	err := handler.HandleEvent(context.Background(), Event{
		EventType: "IndexArticle",
		EventID:   "evt-2",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	handler.Stop()

	if len(se.indexedDocs) != 1 {
		t.Errorf("expected 1 indexed doc, got %d", len(se.indexedDocs))
	}
}

func TestIndexEventHandler_HandleEvent_UnknownType(t *testing.T) {
	se := &mockSearchEngine{}
	repo := &mockArticleRepo{articles: map[string]*domain.Article{}}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	err := handler.HandleEvent(context.Background(), Event{
		EventType: "UnknownEvent",
		EventID:   "evt-3",
	})
	if err != nil {
		t.Fatalf("HandleEvent() should return nil for unknown events, got %v", err)
	}
}

func TestIndexEventHandler_HandleEvent_InvalidPayload(t *testing.T) {
	se := &mockSearchEngine{}
	repo := &mockArticleRepo{articles: map[string]*domain.Article{}}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	err := handler.HandleEvent(context.Background(), Event{
		EventType: "ArticleCreated",
		EventID:   "evt-4",
		Payload:   json.RawMessage(`{invalid json}`),
	})
	if err == nil {
		t.Fatal("HandleEvent() should return error for invalid payload")
	}
}

func TestIndexEventHandler_BatchFlush(t *testing.T) {
	now := time.Now()
	articles := make(map[string]*domain.Article)
	for i := range batchFlushSize + 2 {
		id := "art-" + string(rune('a'+i))
		a, _ := domain.NewArticle(id, "Title", "Content", []string{}, now, "user")
		articles[id] = a
	}

	repo := &mockArticleRepo{articles: articles}
	se := &mockSearchEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	// Enqueue batchFlushSize items to trigger immediate flush
	for i := range batchFlushSize {
		id := "art-" + string(rune('a'+i))
		payload, _ := json.Marshal(ArticleCreatedPayload{ArticleID: id})
		_ = handler.HandleEvent(context.Background(), Event{
			EventType: "ArticleCreated",
			EventID:   "evt-batch",
			Payload:   payload,
		})
	}

	// Wait a short time for the flush goroutine
	time.Sleep(100 * time.Millisecond)

	if len(se.indexedDocs) != batchFlushSize {
		t.Errorf("expected %d indexed docs after batch flush, got %d", batchFlushSize, len(se.indexedDocs))
	}
}

func TestIndexEventHandler_Deduplication(t *testing.T) {
	now := time.Now()
	article, _ := domain.NewArticle("dup-1", "Title", "Content", []string{}, now, "user")
	repo := &mockArticleRepo{
		articles: map[string]*domain.Article{"dup-1": article},
	}
	se := &mockSearchEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	// Enqueue the same article ID multiple times
	for range 5 {
		payload, _ := json.Marshal(ArticleCreatedPayload{ArticleID: "dup-1"})
		_ = handler.HandleEvent(context.Background(), Event{
			EventType: "ArticleCreated",
			EventID:   "evt-dup",
			Payload:   payload,
		})
	}

	handler.Stop()

	// After deduplication, only 1 document should be indexed
	if len(se.indexedDocs) != 1 {
		t.Errorf("expected 1 indexed doc after deduplication, got %d", len(se.indexedDocs))
	}
}

// fakeAcker records every message ID passed to Ack for assertions. It
// implements the Acknowledger interface consumed by IndexEventHandler.
type fakeAcker struct {
	mu    sync.Mutex
	acked []string
	err   error
}

func (f *fakeAcker) Ack(_ context.Context, messageIDs ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.acked = append(f.acked, messageIDs...)
	return nil
}

func (f *fakeAcker) ackedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.acked))
	copy(out, f.acked)
	return out
}

// TestIndexEventHandler_Flush_AcksMessageIDsOnlyAfterDurableWrite reproduces
// the HIGH finding: HandleEvent used to return nil as soon as an event was
// buffered, and the Redis consumer ACKed on that nil return -- long before
// flush() actually wrote to Meilisearch. This test asserts the handler
// itself withholds the ACK until IndexUsecase.ExecuteBatchArticles has
// durably succeeded.
func TestIndexEventHandler_Flush_AcksMessageIDsOnlyAfterDurableWrite(t *testing.T) {
	now := time.Now()
	article, _ := domain.NewArticle("art-ack-1", "Title", "Content", []string{}, now, "user")
	repo := &mockArticleRepo{articles: map[string]*domain.Article{"art-ack-1": article}}
	se := &mockSearchEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	acker := &fakeAcker{}
	handler.SetAcker(acker)

	payload, _ := json.Marshal(ArticleCreatedPayload{ArticleID: "art-ack-1"})
	err := handler.HandleEvent(context.Background(), Event{
		EventType: "ArticleCreated",
		EventID:   "evt-ack-1",
		MessageID: "1-0",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	// Before the batch timer fires, the message must not be ACKed yet --
	// the write hasn't happened.
	if got := acker.ackedIDs(); len(got) != 0 {
		t.Fatalf("message ACKed before flush ran: %v", got)
	}

	handler.Stop()

	got := acker.ackedIDs()
	if len(got) != 1 || got[0] != "1-0" {
		t.Fatalf("acked IDs = %v, want [\"1-0\"] after a successful flush", got)
	}
	if len(se.indexedDocs) != 1 {
		t.Fatalf("expected 1 indexed doc, got %d", len(se.indexedDocs))
	}
}

// TestIndexEventHandler_Flush_DoesNotAckOnFailure ensures a flush failure
// (e.g. Meilisearch unreachable) leaves the message un-ACKed so it remains
// in the stream's pending entries list and is retried by the consumer's
// XAUTOCLAIM reclaim loop instead of being silently lost.
func TestIndexEventHandler_Flush_DoesNotAckOnFailure(t *testing.T) {
	repo := &mockArticleRepo{articles: map[string]*domain.Article{}, err: errors.New("db unavailable")}
	se := &mockSearchEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	acker := &fakeAcker{}
	handler.SetAcker(acker)

	payload, _ := json.Marshal(ArticleCreatedPayload{ArticleID: "art-ack-2"})
	err := handler.HandleEvent(context.Background(), Event{
		EventType: "ArticleCreated",
		EventID:   "evt-ack-2",
		MessageID: "2-0",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	handler.Stop()

	if got := acker.ackedIDs(); len(got) != 0 {
		t.Fatalf("message ACKed despite flush failure: %v", got)
	}
}

// TestIndexEventHandler_HandleEvent_UnknownType_AcksImmediately verifies an
// unroutable event type is ACKed right away (nothing is buffered for it),
// so it doesn't sit in the PEL forever and eventually get mistaken for a
// poison message once a real Acknowledger is wired.
func TestIndexEventHandler_HandleEvent_UnknownType_AcksImmediately(t *testing.T) {
	se := &mockSearchEngine{}
	repo := &mockArticleRepo{articles: map[string]*domain.Article{}}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	acker := &fakeAcker{}
	handler.SetAcker(acker)

	err := handler.HandleEvent(context.Background(), Event{
		EventType: "UnknownEvent",
		EventID:   "evt-unknown",
		MessageID: "3-0",
	})
	if err != nil {
		t.Fatalf("HandleEvent() should return nil for unknown events, got %v", err)
	}

	if got := acker.ackedIDs(); len(got) != 1 || got[0] != "3-0" {
		t.Fatalf("acked IDs = %v, want [\"3-0\"] immediately for an unknown event type", got)
	}
}
