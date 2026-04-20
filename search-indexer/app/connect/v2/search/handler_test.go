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
func (m *mockSearchEngine) SearchWithDateFilter(ctx context.Context, query string, publishedAfter, publishedBefore *time.Time, limit int) ([]domain.SearchDocument, error) {
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

// mockRecapSearchEngine implements port.RecapSearchEngine for testing.
type mockRecapSearchEngine struct {
	docs           []domain.RecapDocument
	estimatedTotal int64
	err            error
	lastQuery      string
	lastLimit      int
}

func (m *mockRecapSearchEngine) EnsureRecapIndex(ctx context.Context) error { return m.err }
func (m *mockRecapSearchEngine) IndexRecapDocuments(ctx context.Context, docs []domain.RecapDocument) error {
	return m.err
}
func (m *mockRecapSearchEngine) SearchRecaps(ctx context.Context, query string, limit int) ([]domain.RecapDocument, int64, error) {
	m.lastQuery = query
	m.lastLimit = limit
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.docs, m.estimatedTotal, nil
}

var _ port.RecapSearchEngine = (*mockRecapSearchEngine)(nil)

func TestHandler_SearchArticles_Success(t *testing.T) {
	now := time.Now()
	article, _ := domain.NewArticle("1", "Test Title", "Test Content", []string{"go", "test"}, now, "user1")
	doc := domain.NewSearchDocument(article)

	se := &mockSearchEngine{
		docs:           []domain.SearchDocument{doc},
		estimatedTotal: 1,
	}
	uc := usecase.NewSearchByUserUsecase(se)
	handler := NewHandler(uc, nil)

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
	handler := NewHandler(uc, nil)

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
	handler := NewHandler(uc, nil)

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
	handler := NewHandler(uc, nil)

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
	handler := NewHandler(uc, nil)

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

func TestHandler_SearchRecaps_WithQuery(t *testing.T) {
	recapEngine := &mockRecapSearchEngine{
		docs: []domain.RecapDocument{
			{
				ID:       "job1__tech",
				JobID:    "job1",
				Genre:    "tech",
				Summary:  "Technology recap",
				TopTerms: []string{"ai", "golang"},
				Tags:     []string{"artificial-intelligence"},
				Bullets:  []string{"bullet1"},
			},
		},
		estimatedTotal: 1,
	}
	recapUC := usecase.NewSearchRecapsUsecase(recapEngine)
	handler := NewHandler(nil, recapUC)

	query := "technology ai"
	req := connect.NewRequest(&searchv2.SearchRecapsRequest{
		Query: &query,
		Limit: 10,
	})

	resp, err := handler.SearchRecaps(context.Background(), req)
	if err != nil {
		t.Fatalf("SearchRecaps() error = %v", err)
	}

	if len(resp.Msg.Hits) != 1 {
		t.Fatalf("Hits count = %d, want 1", len(resp.Msg.Hits))
	}
	if resp.Msg.Hits[0].Genre != "tech" {
		t.Errorf("Genre = %q, want %q", resp.Msg.Hits[0].Genre, "tech")
	}
	if resp.Msg.EstimatedTotalHits != 1 {
		t.Errorf("EstimatedTotalHits = %d, want 1", resp.Msg.EstimatedTotalHits)
	}
	// Verify the query was passed to the search engine
	if recapEngine.lastQuery != "technology ai" {
		t.Errorf("Search engine received query = %q, want %q", recapEngine.lastQuery, "technology ai")
	}
}

func TestHandler_SearchRecaps_QueryTakesPrecedenceOverTagName(t *testing.T) {
	recapEngine := &mockRecapSearchEngine{
		docs:           []domain.RecapDocument{},
		estimatedTotal: 0,
	}
	recapUC := usecase.NewSearchRecapsUsecase(recapEngine)
	handler := NewHandler(nil, recapUC)

	query := "free text search"
	req := connect.NewRequest(&searchv2.SearchRecapsRequest{
		TagName: "some-tag",
		Query:   &query,
		Limit:   10,
	})

	_, err := handler.SearchRecaps(context.Background(), req)
	if err != nil {
		t.Fatalf("SearchRecaps() error = %v", err)
	}

	// When both query and tag_name are set, query takes precedence
	if recapEngine.lastQuery != "free text search" {
		t.Errorf("Search engine received query = %q, want %q (query should take precedence over tag_name)", recapEngine.lastQuery, "free text search")
	}
}

func TestHandler_SearchRecaps_FallbackToTagName(t *testing.T) {
	recapEngine := &mockRecapSearchEngine{
		docs:           []domain.RecapDocument{},
		estimatedTotal: 0,
	}
	recapUC := usecase.NewSearchRecapsUsecase(recapEngine)
	handler := NewHandler(nil, recapUC)

	req := connect.NewRequest(&searchv2.SearchRecapsRequest{
		TagName: "golang",
		Limit:   10,
	})

	_, err := handler.SearchRecaps(context.Background(), req)
	if err != nil {
		t.Fatalf("SearchRecaps() error = %v", err)
	}

	// When query is not set, tag_name is used
	if recapEngine.lastQuery != "golang" {
		t.Errorf("Search engine received query = %q, want %q (should fall back to tag_name)", recapEngine.lastQuery, "golang")
	}
}

func TestHandler_SearchRecaps_NeitherQueryNorTagName(t *testing.T) {
	recapEngine := &mockRecapSearchEngine{}
	recapUC := usecase.NewSearchRecapsUsecase(recapEngine)
	handler := NewHandler(nil, recapUC)

	req := connect.NewRequest(&searchv2.SearchRecapsRequest{
		Limit: 10,
	})

	_, err := handler.SearchRecaps(context.Background(), req)
	if err == nil {
		t.Fatal("SearchRecaps() should return error when neither query nor tag_name is set")
	}

	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Error code = %v, want InvalidArgument", connectErr.Code())
	}
}
