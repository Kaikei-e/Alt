package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"search-indexer/domain"
	"search-indexer/port"
	"search-indexer/usecase"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ikawaha/kagome/v2/tokenizer"
)

// mockArticleRepo implements port.ArticleRepository for testing.
type mockArticleRepo struct {
	articles map[string]*domain.Article
	err      error
	// errByID fails only the listed article IDs, modelling a transient
	// backend problem that affects part of a batch rather than all of it.
	errByID map[string]error
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
	if err, ok := m.errByID[articleID]; ok {
		return nil, err
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

// ArticleCreated and ArticleUpdated name an article, they do not carry it:
// both resolve the body from alt-backend by article_id. alt-backend still
// embeds content/tags in the payload while the producer is migrated, so this
// asserts the embedded copy is ignored -- indexing it would make the search
// index a function of the event rather than of the article, and it is the
// embedded body that drove alt:events:articles to 973 MB.
//
// ArticleUpdated in particular fell through the default branch before it had
// a case here, and the index silently went stale on every article edit.
func TestIndexEventHandler_HandleEvent_ArticleEvents_IndexFetchedBody(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		articleID string
	}{
		{name: "ArticleCreated", eventType: "ArticleCreated", articleID: "art-created-1"},
		{name: "ArticleUpdated", eventType: "ArticleUpdated", articleID: "art-updated-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdAt := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
			article, _ := domain.NewArticle(tt.articleID, "Canonical Title", "Canonical body from alt-backend", []string{"go"}, createdAt, "user-1")
			repo := &mockArticleRepo{articles: map[string]*domain.Article{tt.articleID: article}}
			se := &mockSearchEngine{}
			uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
			handler := NewIndexEventHandler(uc, slog.Default())
			defer handler.Stop()

			// Raw JSON rather than ArticleCreatedPayload: the struct has no
			// content/tags fields, but the wire payload still does.
			payload := []byte(`{"article_id":"` + tt.articleID + `",` +
				`"user_id":"user-1","feed_id":"feed-1","title":"Stale Payload Title",` +
				`"url":"https://example.com/article","content":"Stale payload body",` +
				`"tags":["stale"],"published_at":"2026-03-26T00:00:00Z"}`)

			err := handler.HandleEvent(context.Background(), Event{
				EventType: tt.eventType,
				EventID:   "evt-" + tt.articleID,
				Payload:   payload,
			})
			if err != nil {
				t.Fatalf("HandleEvent(%s) error = %v", tt.eventType, err)
			}

			handler.Stop()

			if len(se.indexedDocs) != 1 {
				t.Fatalf("expected 1 indexed doc for %s, got %d", tt.eventType, len(se.indexedDocs))
			}
			doc := se.indexedDocs[0]
			if doc.ID != tt.articleID {
				t.Errorf("indexed doc ID = %q, want %q", doc.ID, tt.articleID)
			}
			if doc.Content != "Canonical body from alt-backend" {
				t.Errorf("indexed content = %q, want the body fetched from alt-backend", doc.Content)
			}
			if doc.Title != "Canonical Title" {
				t.Errorf("indexed title = %q, want the title fetched from alt-backend", doc.Title)
			}
		})
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

	// Deterministically wait for the flush goroutine to signal completion
	// via the handler's own flushed channel instead of guessing a sleep
	// duration (a slow CI runner could flush after 100ms and flake this
	// test into a false failure).
	select {
	case <-handler.flushed:
	case <-time.After(2 * time.Second):
		t.Fatal("flush did not complete within 2s")
	}

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

// TestIndexEventHandler_Flush_PartialFailureAcksOnlyIndexedMessages is the
// consumer-side half of the batch-indexing HIGH finding. Each Redis Stream
// message names exactly one article, so ACK granularity can follow indexing
// granularity: one transient backend failure inside a 10-message batch must
// cost exactly one un-ACKed message, not ten. Before this, ExecuteBatchArticles
// aborted on the first non-not-found error and flush() ACKed nothing, so all
// ten messages were redelivered, all ten burned a delivery attempt against
// MaxDeliveries, and a backend outage lasting a few reaper intervals pushed
// perfectly healthy messages into a DLQ no code in this repository reads.
func TestIndexEventHandler_Flush_PartialFailureAcksOnlyIndexedMessages(t *testing.T) {
	now := time.Now()
	articles := make(map[string]*domain.Article, batchFlushSize)
	for i := range batchFlushSize {
		id := fmt.Sprintf("art-%d", i)
		a, err := domain.NewArticle(id, "Title", "Content", nil, now.Add(time.Duration(i)*time.Second), "user")
		if err != nil {
			t.Fatalf("NewArticle(%s): %v", id, err)
		}
		articles[id] = a
	}
	delete(articles, "art-4")

	repo := &mockArticleRepo{
		articles: articles,
		errByID: map[string]error{
			"art-4": &domain.RepositoryError{Op: "GetArticleByID", Err: errors.New("connection pool exhausted")},
		},
	}
	se := &mockSearchEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	acker := &fakeAcker{}
	handler.SetAcker(acker)

	for i := range batchFlushSize {
		payload, _ := json.Marshal(ArticleCreatedPayload{ArticleID: fmt.Sprintf("art-%d", i)})
		if err := handler.HandleEvent(context.Background(), Event{
			EventType: "ArticleCreated",
			EventID:   fmt.Sprintf("evt-%d", i),
			MessageID: fmt.Sprintf("%d-0", i),
			Payload:   payload,
		}); err != nil {
			t.Fatalf("HandleEvent(%d) error = %v", i, err)
		}
	}

	handler.Stop()

	acked := acker.ackedIDs()
	slices.Sort(acked)
	want := []string{"0-0", "1-0", "2-0", "3-0", "5-0", "6-0", "7-0", "8-0", "9-0"}
	if !slices.Equal(acked, want) {
		t.Fatalf("acked IDs = %v, want %v (the 9 healthy messages ACKed, only the failing article's message left pending)", acked, want)
	}
	if len(se.indexedDocs) != batchFlushSize-1 {
		t.Fatalf("indexed docs = %d, want %d (healthy articles must not be dropped alongside the failing one)", len(se.indexedDocs), batchFlushSize-1)
	}
}

// TestIndexEventHandler_Flush_AcksMessagesForDeletedArticles covers the
// terminal half of the same split: an article that no longer exists upstream
// can never be indexed, so leaving its message un-ACKed would just burn
// delivery attempts until the reaper routed it to the DLQ. It must be ACKed
// exactly like a successfully indexed one.
func TestIndexEventHandler_Flush_AcksMessagesForDeletedArticles(t *testing.T) {
	now := time.Now()
	kept, err := domain.NewArticle("art-kept", "Title", "Content", nil, now, "user")
	if err != nil {
		t.Fatalf("NewArticle: %v", err)
	}
	repo := &mockArticleRepo{
		articles: map[string]*domain.Article{"art-kept": kept},
		errByID:  map[string]error{"art-deleted": domain.ErrArticleNotFound},
	}
	se := &mockSearchEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	acker := &fakeAcker{}
	handler.SetAcker(acker)

	for i, id := range []string{"art-kept", "art-deleted"} {
		payload, _ := json.Marshal(ArticleCreatedPayload{ArticleID: id})
		if err := handler.HandleEvent(context.Background(), Event{
			EventType: "ArticleCreated",
			EventID:   "evt-" + id,
			MessageID: fmt.Sprintf("%d-0", i),
			Payload:   payload,
		}); err != nil {
			t.Fatalf("HandleEvent(%s) error = %v", id, err)
		}
	}

	handler.Stop()

	acked := acker.ackedIDs()
	slices.Sort(acked)
	if !slices.Equal(acked, []string{"0-0", "1-0"}) {
		t.Fatalf("acked IDs = %v, want [0-0 1-0]: a deleted article is terminal, not retryable", acked)
	}
}

// TestIndexEventHandler_Flush_AcksEveryMessageNamingASucceededArticle guards
// the dedup path: flush() collapses duplicate article IDs into one usecase
// call, so the per-article outcome must fan back out to every message that
// named it. Missing that would leave duplicate messages pending forever and
// eventually DLQ them despite the article being indexed.
func TestIndexEventHandler_Flush_AcksEveryMessageNamingASucceededArticle(t *testing.T) {
	now := time.Now()
	article, err := domain.NewArticle("dup-1", "Title", "Content", nil, now, "user")
	if err != nil {
		t.Fatalf("NewArticle: %v", err)
	}
	repo := &mockArticleRepo{articles: map[string]*domain.Article{"dup-1": article}}
	se := &mockSearchEngine{}
	uc := usecase.NewIndexArticlesUsecase(repo, se, (*tokenizer.Tokenizer)(nil))
	handler := NewIndexEventHandler(uc, slog.Default())
	defer handler.Stop()

	acker := &fakeAcker{}
	handler.SetAcker(acker)

	for i := range 3 {
		payload, _ := json.Marshal(ArticleCreatedPayload{ArticleID: "dup-1"})
		if err := handler.HandleEvent(context.Background(), Event{
			EventType: "ArticleCreated",
			EventID:   "evt-dup",
			MessageID: fmt.Sprintf("%d-0", i),
			Payload:   payload,
		}); err != nil {
			t.Fatalf("HandleEvent(%d) error = %v", i, err)
		}
	}

	handler.Stop()

	acked := acker.ackedIDs()
	slices.Sort(acked)
	if !slices.Equal(acked, []string{"0-0", "1-0", "2-0"}) {
		t.Fatalf("acked IDs = %v, want all three messages naming the indexed article", acked)
	}
	if len(se.indexedDocs) != 1 {
		t.Fatalf("indexed docs = %d, want 1 (dedup still applies to the write)", len(se.indexedDocs))
	}
}
