package backend_api

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"pre-processor/domain"
	backendv1 "pre-processor/gen/proto/clients/preprocessor-backend/v1"
	"pre-processor/gen/proto/clients/preprocessor-backend/v1/backendv1connect"
)

// mockBackendClient implements backendv1connect.BackendInternalServiceClient for testing.
type mockBackendClient struct {
	backendv1connect.UnimplementedBackendInternalServiceHandler

	getFeedIDFunc              func(ctx context.Context, req *connect.Request[backendv1.GetFeedIDRequest]) (*connect.Response[backendv1.GetFeedIDResponse], error)
	createArticleFunc          func(ctx context.Context, req *connect.Request[backendv1.CreateArticleRequest]) (*connect.Response[backendv1.CreateArticleResponse], error)
	listUnsummarizedFunc       func(ctx context.Context, req *connect.Request[backendv1.ListUnsummarizedArticlesRequest]) (*connect.Response[backendv1.ListUnsummarizedArticlesResponse], error)
	hasUnsummarizedFunc        func(ctx context.Context, req *connect.Request[backendv1.HasUnsummarizedArticlesRequest]) (*connect.Response[backendv1.HasUnsummarizedArticlesResponse], error)
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

func (m *mockBackendClient) ListUnsummarizedArticles(ctx context.Context, req *connect.Request[backendv1.ListUnsummarizedArticlesRequest]) (*connect.Response[backendv1.ListUnsummarizedArticlesResponse], error) {
	if m.listUnsummarizedFunc != nil {
		return m.listUnsummarizedFunc(ctx, req)
	}
	return connect.NewResponse(&backendv1.ListUnsummarizedArticlesResponse{}), nil
}

func (m *mockBackendClient) HasUnsummarizedArticles(ctx context.Context, req *connect.Request[backendv1.HasUnsummarizedArticlesRequest]) (*connect.Response[backendv1.HasUnsummarizedArticlesResponse], error) {
	if m.hasUnsummarizedFunc != nil {
		return m.hasUnsummarizedFunc(ctx, req)
	}
	return connect.NewResponse(&backendv1.HasUnsummarizedArticlesResponse{}), nil
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

// ── FindForSummarization tests ──

func TestFindForSummarization_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	mock := &mockBackendClient{
		listUnsummarizedFunc: func(_ context.Context, req *connect.Request[backendv1.ListUnsummarizedArticlesRequest]) (*connect.Response[backendv1.ListUnsummarizedArticlesResponse], error) {
			if req.Msg.Limit != 10 {
				t.Errorf("expected limit 10, got %d", req.Msg.Limit)
			}
			return connect.NewResponse(&backendv1.ListUnsummarizedArticlesResponse{
				Articles: []*backendv1.UnsummarizedArticle{
					{Id: "a1", Title: "T1", Content: "C1", Url: "http://ex.com/1", CreatedAt: timestamppb.New(now), UserId: "u1"},
					{Id: "a2", Title: "T2", Content: "C2", Url: "http://ex.com/2", CreatedAt: timestamppb.New(now.Add(-time.Hour)), UserId: "u1"},
				},
				NextCreatedAt: timestamppb.New(now.Add(-time.Hour)),
				NextId:        "a2",
			}), nil
		},
	}
	repo := newTestRepo(mock)

	articles, cursor, err := repo.FindForSummarization(context.Background(), nil, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(articles))
	}
	if articles[0].ID != "a1" {
		t.Errorf("expected first article ID a1, got %s", articles[0].ID)
	}
	if articles[0].URL != "http://ex.com/1" {
		t.Errorf("expected URL http://ex.com/1, got %s", articles[0].URL)
	}
	if cursor == nil {
		t.Fatal("expected cursor to be set")
	}
	if cursor.LastID != "a2" {
		t.Errorf("expected cursor LastID a2, got %s", cursor.LastID)
	}
}

func TestFindForSummarization_WithCursor(t *testing.T) {
	cursorTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	mock := &mockBackendClient{
		listUnsummarizedFunc: func(_ context.Context, req *connect.Request[backendv1.ListUnsummarizedArticlesRequest]) (*connect.Response[backendv1.ListUnsummarizedArticlesResponse], error) {
			if req.Msg.LastId != "prev-id" {
				t.Errorf("expected last_id prev-id, got %s", req.Msg.LastId)
			}
			if req.Msg.LastCreatedAt == nil {
				t.Fatal("expected last_created_at to be set")
			}
			return connect.NewResponse(&backendv1.ListUnsummarizedArticlesResponse{}), nil
		},
	}
	repo := newTestRepo(mock)

	cursor := &domain.Cursor{LastCreatedAt: &cursorTime, LastID: "prev-id"}
	articles, nextCursor, err := repo.FindForSummarization(context.Background(), cursor, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(articles) != 0 {
		t.Fatalf("expected 0 articles, got %d", len(articles))
	}
	if nextCursor != nil {
		t.Error("expected nil cursor for empty response")
	}
}

func TestFindForSummarization_Error(t *testing.T) {
	mock := &mockBackendClient{
		listUnsummarizedFunc: func(_ context.Context, req *connect.Request[backendv1.ListUnsummarizedArticlesRequest]) (*connect.Response[backendv1.ListUnsummarizedArticlesResponse], error) {
			return nil, connect.NewError(connect.CodeInternal, errors.New("server error"))
		},
	}
	repo := newTestRepo(mock)

	_, _, err := repo.FindForSummarization(context.Background(), nil, 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── HasUnsummarizedArticles tests ──

func TestHasUnsummarizedArticles_True(t *testing.T) {
	mock := &mockBackendClient{
		hasUnsummarizedFunc: func(_ context.Context, req *connect.Request[backendv1.HasUnsummarizedArticlesRequest]) (*connect.Response[backendv1.HasUnsummarizedArticlesResponse], error) {
			return connect.NewResponse(&backendv1.HasUnsummarizedArticlesResponse{HasUnsummarized: true}), nil
		},
	}
	repo := newTestRepo(mock)

	has, err := repo.HasUnsummarizedArticles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected true, got false")
	}
}

func TestHasUnsummarizedArticles_False(t *testing.T) {
	mock := &mockBackendClient{
		hasUnsummarizedFunc: func(_ context.Context, req *connect.Request[backendv1.HasUnsummarizedArticlesRequest]) (*connect.Response[backendv1.HasUnsummarizedArticlesResponse], error) {
			return connect.NewResponse(&backendv1.HasUnsummarizedArticlesResponse{HasUnsummarized: false}), nil
		},
	}
	repo := newTestRepo(mock)

	has, err := repo.HasUnsummarizedArticles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected false, got true")
	}
}

func TestHasUnsummarizedArticles_Error(t *testing.T) {
	mock := &mockBackendClient{
		hasUnsummarizedFunc: func(_ context.Context, req *connect.Request[backendv1.HasUnsummarizedArticlesRequest]) (*connect.Response[backendv1.HasUnsummarizedArticlesResponse], error) {
			return nil, connect.NewError(connect.CodeInternal, errors.New("server error"))
		},
	}
	repo := newTestRepo(mock)

	_, err := repo.HasUnsummarizedArticles(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
