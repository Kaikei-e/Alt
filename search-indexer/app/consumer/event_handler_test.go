package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"search-indexer/domain"
	"search-indexer/port"
	"search-indexer/usecase"
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
	return nil, &domain.RepositoryError{Op: "GetArticleByID", Err: "not found"}
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
