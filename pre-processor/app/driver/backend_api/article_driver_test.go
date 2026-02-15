package backend_api

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"

	"pre-processor/domain"
	backendv1 "pre-processor/gen/proto/clients/preprocessor-backend/v1"
	"pre-processor/gen/proto/clients/preprocessor-backend/v1/backendv1connect"
)

// mockBackendClient implements backendv1connect.BackendInternalServiceClient for testing.
type mockBackendClient struct {
	backendv1connect.UnimplementedBackendInternalServiceHandler

	getFeedIDFunc     func(ctx context.Context, req *connect.Request[backendv1.GetFeedIDRequest]) (*connect.Response[backendv1.GetFeedIDResponse], error)
	createArticleFunc func(ctx context.Context, req *connect.Request[backendv1.CreateArticleRequest]) (*connect.Response[backendv1.CreateArticleResponse], error)
}

func (m *mockBackendClient) GetFeedID(ctx context.Context, req *connect.Request[backendv1.GetFeedIDRequest]) (*connect.Response[backendv1.GetFeedIDResponse], error) {
	if m.getFeedIDFunc != nil {
		return m.getFeedIDFunc(ctx, req)
	}
	return connect.NewResponse(&backendv1.GetFeedIDResponse{}), nil
}

func (m *mockBackendClient) CreateArticle(ctx context.Context, req *connect.Request[backendv1.CreateArticleRequest]) (*connect.Response[backendv1.CreateArticleResponse], error) {
	if m.createArticleFunc != nil {
		return m.createArticleFunc(ctx, req)
	}
	return connect.NewResponse(&backendv1.CreateArticleResponse{ArticleId: "test-id"}), nil
}

func newTestRepo(mock *mockBackendClient) *ArticleRepository {
	client := &Client{client: mock}
	return NewArticleRepository(client, nil)
}

func TestFetchInoreaderArticles_NilDBPool(t *testing.T) {
	client := &Client{} // dummy client, not used for this method
	repo := NewArticleRepository(client, nil)

	_, err := repo.FetchInoreaderArticles(context.Background(), time.Now().Add(-1*time.Hour))
	if err == nil {
		t.Fatal("expected error when dbPool is nil, got nil")
	}

	want := "database connection is nil"
	if err.Error() != want {
		t.Errorf("got error %q, want %q", err.Error(), want)
	}
}

func TestFetchInoreaderArticles_DBPoolFieldIsSet(t *testing.T) {
	client := &Client{}
	repo := NewArticleRepository(client, nil)

	// Verify the struct stores the dbPool (nil in this case)
	if repo.dbPool != nil {
		t.Error("expected dbPool to be nil")
	}
}

func TestUpsertArticles_EmptySlice(t *testing.T) {
	mock := &mockBackendClient{}
	repo := newTestRepo(mock)

	err := repo.UpsertArticles(context.Background(), []*domain.Article{})
	if err != nil {
		t.Fatalf("expected nil error for empty slice, got %v", err)
	}
}

func TestUpsertArticles_SkipsEmptyFeedURL(t *testing.T) {
	var createCalled int
	mock := &mockBackendClient{
		createArticleFunc: func(_ context.Context, req *connect.Request[backendv1.CreateArticleRequest]) (*connect.Response[backendv1.CreateArticleResponse], error) {
			createCalled++
			return connect.NewResponse(&backendv1.CreateArticleResponse{ArticleId: "new-id"}), nil
		},
		getFeedIDFunc: func(_ context.Context, req *connect.Request[backendv1.GetFeedIDRequest]) (*connect.Response[backendv1.GetFeedIDResponse], error) {
			return connect.NewResponse(&backendv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
		},
	}
	repo := newTestRepo(mock)

	articles := []*domain.Article{
		{URL: "https://example.com/no-feed", FeedURL: "", FeedID: ""},       // should be skipped
		{URL: "https://example.com/has-feed", FeedURL: "https://feed.com"}, // should be created
	}

	err := repo.UpsertArticles(context.Background(), articles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createCalled != 1 {
		t.Errorf("expected CreateArticle called 1 time, got %d", createCalled)
	}
}

func TestUpsertArticles_SkipsFeedNotFound(t *testing.T) {
	var createCalled int
	mock := &mockBackendClient{
		createArticleFunc: func(_ context.Context, req *connect.Request[backendv1.CreateArticleRequest]) (*connect.Response[backendv1.CreateArticleResponse], error) {
			createCalled++
			return connect.NewResponse(&backendv1.CreateArticleResponse{ArticleId: "new-id"}), nil
		},
		getFeedIDFunc: func(_ context.Context, req *connect.Request[backendv1.GetFeedIDRequest]) (*connect.Response[backendv1.GetFeedIDResponse], error) {
			if req.Msg.FeedUrl == "https://unknown-feed.com" {
				return nil, connect.NewError(connect.CodeNotFound, errors.New("feed not found"))
			}
			return connect.NewResponse(&backendv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
		},
	}
	repo := newTestRepo(mock)

	articles := []*domain.Article{
		{URL: "https://example.com/unknown", FeedURL: "https://unknown-feed.com"}, // feed not found — skip
		{URL: "https://example.com/known", FeedURL: "https://known-feed.com"},     // feed found — create
	}

	err := repo.UpsertArticles(context.Background(), articles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createCalled != 1 {
		t.Errorf("expected CreateArticle called 1 time, got %d", createCalled)
	}
}

func TestUpsertArticles_CreateErrorAbortsBatch(t *testing.T) {
	mock := &mockBackendClient{
		createArticleFunc: func(_ context.Context, req *connect.Request[backendv1.CreateArticleRequest]) (*connect.Response[backendv1.CreateArticleResponse], error) {
			return nil, connect.NewError(connect.CodeInternal, errors.New("network failure"))
		},
		getFeedIDFunc: func(_ context.Context, req *connect.Request[backendv1.GetFeedIDRequest]) (*connect.Response[backendv1.GetFeedIDResponse], error) {
			return connect.NewResponse(&backendv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
		},
	}
	repo := newTestRepo(mock)

	articles := []*domain.Article{
		{URL: "https://example.com/article1", FeedURL: "https://feed.com"},
	}

	err := repo.UpsertArticles(context.Background(), articles)
	if err == nil {
		t.Fatal("expected error from CreateArticle failure, got nil")
	}
}

func TestUpsertArticles_WithPresetFeedID(t *testing.T) {
	var createCalled int
	var getFeedIDCalled int
	mock := &mockBackendClient{
		createArticleFunc: func(_ context.Context, req *connect.Request[backendv1.CreateArticleRequest]) (*connect.Response[backendv1.CreateArticleResponse], error) {
			createCalled++
			if req.Msg.FeedId != "preset-feed" {
				return nil, errors.New("expected preset-feed as FeedId")
			}
			return connect.NewResponse(&backendv1.CreateArticleResponse{ArticleId: "new-id"}), nil
		},
		getFeedIDFunc: func(_ context.Context, req *connect.Request[backendv1.GetFeedIDRequest]) (*connect.Response[backendv1.GetFeedIDResponse], error) {
			getFeedIDCalled++
			return connect.NewResponse(&backendv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
		},
	}
	repo := newTestRepo(mock)

	articles := []*domain.Article{
		{URL: "https://example.com/article1", FeedID: "preset-feed"},
	}

	err := repo.UpsertArticles(context.Background(), articles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createCalled != 1 {
		t.Errorf("expected CreateArticle called 1 time, got %d", createCalled)
	}
	if getFeedIDCalled != 0 {
		t.Errorf("expected GetFeedID not called when FeedID is preset, got %d calls", getFeedIDCalled)
	}
}
