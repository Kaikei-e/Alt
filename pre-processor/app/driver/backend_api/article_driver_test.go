package backend_api

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"pre-processor/domain"
	datahubv1 "pre-processor/gen/proto/alt/datahub/v1"
	"pre-processor/gen/proto/alt/datahub/v1/datahubv1connect"
)

// mockDataHubClient implements datahubv1connect.DataHubServiceClient for testing.
type mockDataHubClient struct {
	datahubv1connect.UnimplementedDataHubServiceHandler

	getFeedIDFunc        func(ctx context.Context, req *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error)
	createArticleFunc    func(ctx context.Context, req *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error)
	listUnsummarizedFunc func(ctx context.Context, req *connect.Request[datahubv1.ListUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.ListUnsummarizedArticlesResponse], error)
	hasUnsummarizedFunc  func(ctx context.Context, req *connect.Request[datahubv1.HasUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.HasUnsummarizedArticlesResponse], error)
	getEmptyFeedIDFunc   func(ctx context.Context, req *connect.Request[datahubv1.GetEmptyFeedIDRequest]) (*connect.Response[datahubv1.GetEmptyFeedIDResponse], error)
}

func (m *mockDataHubClient) GetFeedID(ctx context.Context, req *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
	if m.getFeedIDFunc != nil {
		return m.getFeedIDFunc(ctx, req)
	}
	return connect.NewResponse(&datahubv1.GetFeedIDResponse{}), nil
}

func (m *mockDataHubClient) CreateArticle(ctx context.Context, req *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error) {
	if m.createArticleFunc != nil {
		return m.createArticleFunc(ctx, req)
	}
	return connect.NewResponse(&datahubv1.CreateArticleResponse{ArticleId: "test-id"}), nil
}

func (m *mockDataHubClient) ListUnsummarizedArticles(ctx context.Context, req *connect.Request[datahubv1.ListUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.ListUnsummarizedArticlesResponse], error) {
	if m.listUnsummarizedFunc != nil {
		return m.listUnsummarizedFunc(ctx, req)
	}
	return connect.NewResponse(&datahubv1.ListUnsummarizedArticlesResponse{}), nil
}

func (m *mockDataHubClient) HasUnsummarizedArticles(ctx context.Context, req *connect.Request[datahubv1.HasUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.HasUnsummarizedArticlesResponse], error) {
	if m.hasUnsummarizedFunc != nil {
		return m.hasUnsummarizedFunc(ctx, req)
	}
	return connect.NewResponse(&datahubv1.HasUnsummarizedArticlesResponse{}), nil
}

func (m *mockDataHubClient) GetEmptyFeedID(ctx context.Context, req *connect.Request[datahubv1.GetEmptyFeedIDRequest]) (*connect.Response[datahubv1.GetEmptyFeedIDResponse], error) {
	if m.getEmptyFeedIDFunc != nil {
		return m.getEmptyFeedIDFunc(ctx, req)
	}
	return connect.NewResponse(&datahubv1.GetEmptyFeedIDResponse{}), nil
}

func newTestRepo(mock *mockDataHubClient) *ArticleRepository {
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
	mock := &mockDataHubClient{}
	repo := newTestRepo(mock)

	err := repo.UpsertArticles(context.Background(), []*domain.Article{})
	if err != nil {
		t.Fatalf("expected nil error for empty slice, got %v", err)
	}
}

func TestUpsertArticles_SkipsEmptyFeedURL(t *testing.T) {
	var createCalled int
	mock := &mockDataHubClient{
		createArticleFunc: func(_ context.Context, req *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error) {
			createCalled++
			return connect.NewResponse(&datahubv1.CreateArticleResponse{ArticleId: "new-id"}), nil
		},
		getFeedIDFunc: func(_ context.Context, req *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
			return connect.NewResponse(&datahubv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
		},
	}
	repo := newTestRepo(mock)

	articles := []*domain.Article{
		{URL: "https://example.com/no-feed", FeedURL: "", FeedID: ""},      // should be skipped
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
	mock := &mockDataHubClient{
		createArticleFunc: func(_ context.Context, req *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error) {
			createCalled++
			return connect.NewResponse(&datahubv1.CreateArticleResponse{ArticleId: "new-id"}), nil
		},
		getFeedIDFunc: func(_ context.Context, req *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
			if req.Msg.FeedUrl == "https://unknown-feed.com" {
				return nil, connect.NewError(connect.CodeNotFound, errors.New("feed not found"))
			}
			return connect.NewResponse(&datahubv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
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
	mock := &mockDataHubClient{
		createArticleFunc: func(_ context.Context, req *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error) {
			return nil, connect.NewError(connect.CodeInternal, errors.New("network failure"))
		},
		getFeedIDFunc: func(_ context.Context, req *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
			return connect.NewResponse(&datahubv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
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

// TestUpsertArticles_CoalescesGetFeedIDPerBatch pins the Pillar 4 fix for
// the 262-line GetFeedID burst observed 2026-05-26 (pre-processor calling
// GetFeedID once per article on a Reddit feed batch). The repository must
// resolve each distinct FeedURL once per batch and reuse the result —
// otherwise a single Inoreader fan-out can saturate alt-backend with
// identical NotFound lookups and bury the actual error in log noise.
func TestUpsertArticles_CoalescesGetFeedIDPerBatch(t *testing.T) {
	const batchSize = 25
	var getFeedIDCalls int
	mock := &mockDataHubClient{
		createArticleFunc: func(_ context.Context, _ *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error) {
			return connect.NewResponse(&datahubv1.CreateArticleResponse{ArticleId: "new"}), nil
		},
		getFeedIDFunc: func(_ context.Context, _ *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
			getFeedIDCalls++
			return connect.NewResponse(&datahubv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
		},
	}
	repo := newTestRepo(mock)

	articles := make([]*domain.Article, 0, batchSize)
	for i := 0; i < batchSize; i++ {
		articles = append(articles, &domain.Article{
			URL:     "https://example.com/article-" + intToStr(i),
			FeedURL: "https://example.com/feed.rss",
		})
	}

	if err := repo.UpsertArticles(context.Background(), articles); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if getFeedIDCalls != 1 {
		t.Errorf("expected GetFeedID called once (per-batch cache), got %d", getFeedIDCalls)
	}
}

// TestUpsertArticles_CoalescesNotFoundPerBatch — a feed_url that returns
// NotFound must also be remembered for the rest of the batch. Pre-fix, 262
// articles for an unregistered feed produced 262 identical GetFeedID errors
// and 262 "feed not found, skipping article" warn lines; the cache collapses
// the lookups even for the negative case.
func TestUpsertArticles_CoalescesNotFoundPerBatch(t *testing.T) {
	const batchSize = 20
	var getFeedIDCalls int
	mock := &mockDataHubClient{
		createArticleFunc: func(_ context.Context, _ *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error) {
			return connect.NewResponse(&datahubv1.CreateArticleResponse{ArticleId: "new"}), nil
		},
		getFeedIDFunc: func(_ context.Context, _ *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
			getFeedIDCalls++
			return nil, connect.NewError(connect.CodeNotFound, errors.New("feed not found"))
		},
	}
	repo := newTestRepo(mock)

	articles := make([]*domain.Article, 0, batchSize)
	for i := 0; i < batchSize; i++ {
		articles = append(articles, &domain.Article{
			URL:     "https://reddit.com/r/webdev/comments/" + intToStr(i),
			FeedURL: "https://www.reddit.com/r/webdev/.rss",
		})
	}

	if err := repo.UpsertArticles(context.Background(), articles); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if getFeedIDCalls != 1 {
		t.Errorf("expected GetFeedID called once even on NotFound (per-batch cache), got %d", getFeedIDCalls)
	}
}

func intToStr(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}

func TestUpsertArticles_WithPresetFeedID(t *testing.T) {
	var createCalled int
	var getFeedIDCalled int
	mock := &mockDataHubClient{
		createArticleFunc: func(_ context.Context, req *connect.Request[datahubv1.CreateArticleRequest]) (*connect.Response[datahubv1.CreateArticleResponse], error) {
			createCalled++
			if req.Msg.FeedId != "preset-feed" {
				return nil, errors.New("expected preset-feed as FeedId")
			}
			return connect.NewResponse(&datahubv1.CreateArticleResponse{ArticleId: "new-id"}), nil
		},
		getFeedIDFunc: func(_ context.Context, req *connect.Request[datahubv1.GetFeedIDRequest]) (*connect.Response[datahubv1.GetFeedIDResponse], error) {
			getFeedIDCalled++
			return connect.NewResponse(&datahubv1.GetFeedIDResponse{FeedId: "feed-1"}), nil
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
	mock := &mockDataHubClient{
		listUnsummarizedFunc: func(_ context.Context, req *connect.Request[datahubv1.ListUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.ListUnsummarizedArticlesResponse], error) {
			if req.Msg.Limit != 10 {
				t.Errorf("expected limit 10, got %d", req.Msg.Limit)
			}
			return connect.NewResponse(&datahubv1.ListUnsummarizedArticlesResponse{
				Articles: []*datahubv1.UnsummarizedArticle{
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
	mock := &mockDataHubClient{
		listUnsummarizedFunc: func(_ context.Context, req *connect.Request[datahubv1.ListUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.ListUnsummarizedArticlesResponse], error) {
			if req.Msg.LastId != "prev-id" {
				t.Errorf("expected last_id prev-id, got %s", req.Msg.LastId)
			}
			if req.Msg.LastCreatedAt == nil {
				t.Fatal("expected last_created_at to be set")
			}
			return connect.NewResponse(&datahubv1.ListUnsummarizedArticlesResponse{}), nil
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
	mock := &mockDataHubClient{
		listUnsummarizedFunc: func(_ context.Context, req *connect.Request[datahubv1.ListUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.ListUnsummarizedArticlesResponse], error) {
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
	mock := &mockDataHubClient{
		hasUnsummarizedFunc: func(_ context.Context, req *connect.Request[datahubv1.HasUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.HasUnsummarizedArticlesResponse], error) {
			return connect.NewResponse(&datahubv1.HasUnsummarizedArticlesResponse{HasUnsummarized: true}), nil
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
	mock := &mockDataHubClient{
		hasUnsummarizedFunc: func(_ context.Context, req *connect.Request[datahubv1.HasUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.HasUnsummarizedArticlesResponse], error) {
			return connect.NewResponse(&datahubv1.HasUnsummarizedArticlesResponse{HasUnsummarized: false}), nil
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
	mock := &mockDataHubClient{
		hasUnsummarizedFunc: func(_ context.Context, req *connect.Request[datahubv1.HasUnsummarizedArticlesRequest]) (*connect.Response[datahubv1.HasUnsummarizedArticlesResponse], error) {
			return nil, connect.NewError(connect.CodeInternal, errors.New("server error"))
		},
	}
	repo := newTestRepo(mock)

	_, err := repo.HasUnsummarizedArticles(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── GetEmptyFeedID helper tests ──

func TestGetEmptyFeedID_Found(t *testing.T) {
	mock := &mockDataHubClient{
		getEmptyFeedIDFunc: func(_ context.Context, req *connect.Request[datahubv1.GetEmptyFeedIDRequest]) (*connect.Response[datahubv1.GetEmptyFeedIDResponse], error) {
			if req.Msg.FeedUrl != "http://example.com/feed.xml" {
				t.Errorf("expected feed_url http://example.com/feed.xml, got %s", req.Msg.FeedUrl)
			}
			return connect.NewResponse(&datahubv1.GetEmptyFeedIDResponse{FeedId: "empty-feed-123"}), nil
		},
	}
	repo := newTestRepo(mock)

	feedID, err := repo.getEmptyFeedID(context.Background(), "http://example.com/feed.xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if feedID != "empty-feed-123" {
		t.Errorf("expected empty-feed-123, got %s", feedID)
	}
}

func TestGetEmptyFeedID_NotFound(t *testing.T) {
	mock := &mockDataHubClient{
		getEmptyFeedIDFunc: func(_ context.Context, req *connect.Request[datahubv1.GetEmptyFeedIDRequest]) (*connect.Response[datahubv1.GetEmptyFeedIDResponse], error) {
			return connect.NewResponse(&datahubv1.GetEmptyFeedIDResponse{FeedId: ""}), nil
		},
	}
	repo := newTestRepo(mock)

	feedID, err := repo.getEmptyFeedID(context.Background(), "http://example.com/feed.xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if feedID != "" {
		t.Errorf("expected empty string, got %s", feedID)
	}
}

func TestGetEmptyFeedID_Error(t *testing.T) {
	mock := &mockDataHubClient{
		getEmptyFeedIDFunc: func(_ context.Context, req *connect.Request[datahubv1.GetEmptyFeedIDRequest]) (*connect.Response[datahubv1.GetEmptyFeedIDResponse], error) {
			return nil, connect.NewError(connect.CodeInternal, errors.New("server error"))
		},
	}
	repo := newTestRepo(mock)

	_, err := repo.getEmptyFeedID(context.Background(), "http://example.com/feed.xml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
