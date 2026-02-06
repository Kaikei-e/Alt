package search

import (
	"context"
	"search-indexer/domain"
	"search-indexer/logger"
	"search-indexer/port"
	"search-indexer/usecase"
	"testing"
	"time"

	"connectrpc.com/connect"

	searchv2 "search-indexer/gen/proto/services/search/v2"
)

func TestMain(m *testing.M) {
	logger.Init()
	m.Run()
}

// mockSearchEngine implements port.SearchEngine for testing.
type mockSearchEngine struct {
	docs            []domain.SearchDocument
	estimatedTotal  int64
	err             error
}

func (m *mockSearchEngine) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	return m.err
}
func (m *mockSearchEngine) DeleteDocuments(ctx context.Context, ids []string) error {
	return m.err
}
func (m *mockSearchEngine) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	return m.docs, m.err
}
func (m *mockSearchEngine) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
	return m.docs, m.err
}
func (m *mockSearchEngine) EnsureIndex(ctx context.Context) error { return m.err }
func (m *mockSearchEngine) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error) {
	return m.docs, m.err
}
func (m *mockSearchEngine) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error) {
	return m.docs, m.estimatedTotal, m.err
}
func (m *mockSearchEngine) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	return m.err
}

var _ port.SearchEngine = (*mockSearchEngine)(nil)

func TestHandler_SearchArticles_Success(t *testing.T) {
	now := time.Now()
	article, _ := domain.NewArticle("1", "Test Title", "Test Content", []string{"go", "test"}, now, "user1")
	doc := domain.NewSearchDocument(article)

	se := &mockSearchEngine{
		docs:           []domain.SearchDocument{doc},
		estimatedTotal: 1,
	}
	uc := usecase.NewSearchByUserUsecase(se)
	handler := NewHandler(uc)

	req := connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  "test",
		UserId: "user1",
		Limit:  10,
	})

	resp, err := handler.SearchArticles(context.Background(), req)
	if err != nil {
		t.Fatalf("SearchArticles() error = %v", err)
	}

	if resp.Msg.Query != "test" {
		t.Errorf("Query = %q, want %q", resp.Msg.Query, "test")
	}
	if len(resp.Msg.Hits) != 1 {
		t.Fatalf("Hits count = %d, want 1", len(resp.Msg.Hits))
	}
	if resp.Msg.Hits[0].Id != "1" {
		t.Errorf("Hit ID = %q, want %q", resp.Msg.Hits[0].Id, "1")
	}
	if resp.Msg.EstimatedTotalHits != 1 {
		t.Errorf("EstimatedTotalHits = %d, want 1", resp.Msg.EstimatedTotalHits)
	}
}

func TestHandler_SearchArticles_EmptyQuery(t *testing.T) {
	se := &mockSearchEngine{}
	uc := usecase.NewSearchByUserUsecase(se)
	handler := NewHandler(uc)

	req := connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  "",
		UserId: "user1",
		Limit:  10,
	})

	_, err := handler.SearchArticles(context.Background(), req)
	if err == nil {
		t.Fatal("SearchArticles() should return error for empty query")
	}

	connectErr := new(connect.Error)
	if !connect.IsNotModifiedError(err) {
		// Check it's an invalid argument error
		if ok := err.(*connect.Error); ok != nil {
			connectErr = ok
		}
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Error code = %v, want InvalidArgument", connectErr.Code())
	}
}

func TestHandler_SearchArticles_EmptyUserID(t *testing.T) {
	se := &mockSearchEngine{}
	uc := usecase.NewSearchByUserUsecase(se)
	handler := NewHandler(uc)

	req := connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  "test",
		UserId: "",
		Limit:  10,
	})

	_, err := handler.SearchArticles(context.Background(), req)
	if err == nil {
		t.Fatal("SearchArticles() should return error for empty user_id")
	}
}

func TestHandler_SearchArticles_SearchError(t *testing.T) {
	se := &mockSearchEngine{
		err: &domain.SearchEngineError{Op: "Search", Err: "search failed"},
	}
	uc := usecase.NewSearchByUserUsecase(se)
	handler := NewHandler(uc)

	req := connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  "test",
		UserId: "user1",
		Limit:  10,
	})

	_, err := handler.SearchArticles(context.Background(), req)
	if err == nil {
		t.Fatal("SearchArticles() should return error on search engine failure")
	}
}

func TestHandler_SearchArticles_NilTags(t *testing.T) {
	// Document with nil tags should return empty slice
	doc := domain.SearchDocument{
		ID:      "1",
		Title:   "Test",
		Content: "Content",
		Tags:    nil,
	}

	se := &mockSearchEngine{
		docs:           []domain.SearchDocument{doc},
		estimatedTotal: 1,
	}
	uc := usecase.NewSearchByUserUsecase(se)
	handler := NewHandler(uc)

	req := connect.NewRequest(&searchv2.SearchArticlesRequest{
		Query:  "test",
		UserId: "user1",
		Limit:  10,
	})

	resp, err := handler.SearchArticles(context.Background(), req)
	if err != nil {
		t.Fatalf("SearchArticles() error = %v", err)
	}

	if resp.Msg.Hits[0].Tags == nil {
		t.Error("Tags should not be nil, expected empty slice")
	}
}
