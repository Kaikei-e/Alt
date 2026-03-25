package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/domain"
	backendv1 "alt/gen/proto/services/backend/v1"
	"alt/mocks"
	"alt/port/event_publisher_port"
	"alt/port/internal_article_port"
	"alt/port/internal_feed_port"
	"alt/port/internal_tag_port"

	"go.uber.org/mock/gomock"
)

func setupHandler(t *testing.T) (
	*Handler,
	*mocks.MockListArticlesWithTagsPort,
	*mocks.MockListArticlesWithTagsForwardPort,
	*mocks.MockListDeletedArticlesPort,
	*mocks.MockGetLatestArticleTimestampPort,
	*mocks.MockGetArticleByIDPort,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	listArticles := mocks.NewMockListArticlesWithTagsPort(ctrl)
	listForward := mocks.NewMockListArticlesWithTagsForwardPort(ctrl)
	listDeleted := mocks.NewMockListDeletedArticlesPort(ctrl)
	getTimestamp := mocks.NewMockGetLatestArticleTimestampPort(ctrl)
	getByID := mocks.NewMockGetArticleByIDPort(ctrl)

	h := NewHandler(listArticles, listForward, listDeleted, getTimestamp, getByID, nil)
	return h, listArticles, listForward, listDeleted, getTimestamp, getByID
}

func TestListArticlesWithTags_Success(t *testing.T) {
	h, mockList, _, _, _, _ := setupHandler(t)
	ctx := context.Background()

	now := time.Now()
	expected := []*internal_article_port.ArticleWithTags{
		{ID: "a1", Title: "Title 1", Content: "Content 1", Tags: []string{"go", "rust"}, CreatedAt: now, UserID: "u1"},
		{ID: "a2", Title: "Title 2", Content: "Content 2", Tags: []string{"python"}, CreatedAt: now.Add(-time.Hour), UserID: "u1"},
	}

	mockList.EXPECT().
		ListArticlesWithTags(gomock.Any(), (*time.Time)(nil), "", 200).
		Return(expected, &now, "a2", nil)

	req := connect.NewRequest(&backendv1.ListArticlesWithTagsRequest{
		Limit: 200,
	})

	resp, err := h.ListArticlesWithTags(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Msg.Articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(resp.Msg.Articles))
	}
	if resp.Msg.Articles[0].Id != "a1" {
		t.Errorf("expected first article ID a1, got %s", resp.Msg.Articles[0].Id)
	}
	if resp.Msg.Articles[0].Tags[0] != "go" {
		t.Errorf("expected first tag 'go', got %s", resp.Msg.Articles[0].Tags[0])
	}
	if resp.Msg.NextId != "a2" {
		t.Errorf("expected next_id a2, got %s", resp.Msg.NextId)
	}
	if resp.Msg.NextCreatedAt == nil {
		t.Fatal("expected next_created_at to be set")
	}
}

func TestListArticlesWithTags_WithCursor(t *testing.T) {
	h, mockList, _, _, _, _ := setupHandler(t)
	ctx := context.Background()

	cursorTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	mockList.EXPECT().
		ListArticlesWithTags(gomock.Any(), gomock.Any(), "prev-id", 100).
		Return([]*internal_article_port.ArticleWithTags{}, (*time.Time)(nil), "", nil)

	req := connect.NewRequest(&backendv1.ListArticlesWithTagsRequest{
		LastCreatedAt: timestamppb.New(cursorTime),
		LastId:        "prev-id",
		Limit:         100,
	})

	resp, err := h.ListArticlesWithTags(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Articles) != 0 {
		t.Fatalf("expected 0 articles, got %d", len(resp.Msg.Articles))
	}
}

func TestListArticlesWithTags_Error(t *testing.T) {
	h, mockList, _, _, _, _ := setupHandler(t)
	ctx := context.Background()

	mockList.EXPECT().
		ListArticlesWithTags(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil, "", errors.New("db error"))

	req := connect.NewRequest(&backendv1.ListArticlesWithTagsRequest{Limit: 200})

	_, err := h.ListArticlesWithTags(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
}

func TestListArticlesWithTagsForward_Success(t *testing.T) {
	h, _, mockForward, _, _, _ := setupHandler(t)
	ctx := context.Background()

	now := time.Now()
	mark := now.Add(-24 * time.Hour)
	expected := []*internal_article_port.ArticleWithTags{
		{ID: "a3", Title: "New Article", Content: "Content", Tags: []string{}, CreatedAt: now, UserID: "u1"},
	}

	mockForward.EXPECT().
		ListArticlesWithTagsForward(gomock.Any(), gomock.Any(), (*time.Time)(nil), "", 200).
		Return(expected, &now, "a3", nil)

	req := connect.NewRequest(&backendv1.ListArticlesWithTagsForwardRequest{
		IncrementalMark: timestamppb.New(mark),
		Limit:           200,
	})

	resp, err := h.ListArticlesWithTagsForward(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(resp.Msg.Articles))
	}
}

func TestListDeletedArticles_Success(t *testing.T) {
	h, _, _, mockDeleted, _, _ := setupHandler(t)
	ctx := context.Background()

	now := time.Now()
	expected := []*internal_article_port.DeletedArticle{
		{ID: "d1", DeletedAt: now},
	}

	mockDeleted.EXPECT().
		ListDeletedArticles(gomock.Any(), (*time.Time)(nil), 200).
		Return(expected, &now, nil)

	req := connect.NewRequest(&backendv1.ListDeletedArticlesRequest{Limit: 200})

	resp, err := h.ListDeletedArticles(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Articles) != 1 {
		t.Fatalf("expected 1 deleted article, got %d", len(resp.Msg.Articles))
	}
	if resp.Msg.Articles[0].Id != "d1" {
		t.Errorf("expected ID d1, got %s", resp.Msg.Articles[0].Id)
	}
}

func TestGetLatestArticleTimestamp_Success(t *testing.T) {
	h, _, _, _, mockTimestamp, _ := setupHandler(t)
	ctx := context.Background()

	now := time.Now()
	mockTimestamp.EXPECT().
		GetLatestArticleTimestamp(gomock.Any()).
		Return(&now, nil)

	req := connect.NewRequest(&backendv1.GetLatestArticleTimestampRequest{})

	resp, err := h.GetLatestArticleTimestamp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.LatestCreatedAt == nil {
		t.Fatal("expected latest_created_at to be set")
	}
}

func TestGetLatestArticleTimestamp_NoArticles(t *testing.T) {
	h, _, _, _, mockTimestamp, _ := setupHandler(t)
	ctx := context.Background()

	mockTimestamp.EXPECT().
		GetLatestArticleTimestamp(gomock.Any()).
		Return((*time.Time)(nil), nil)

	req := connect.NewRequest(&backendv1.GetLatestArticleTimestampRequest{})

	resp, err := h.GetLatestArticleTimestamp(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.LatestCreatedAt != nil {
		t.Error("expected latest_created_at to be nil")
	}
}

func TestGetArticleByID_Success(t *testing.T) {
	h, _, _, _, _, mockGetByID := setupHandler(t)
	ctx := context.Background()

	now := time.Now()
	expected := &internal_article_port.ArticleWithTags{
		ID: "a1", Title: "Test", Content: "Body", Tags: []string{"go"}, CreatedAt: now, UserID: "u1",
	}

	mockGetByID.EXPECT().
		GetArticleByID(gomock.Any(), "a1").
		Return(expected, nil)

	req := connect.NewRequest(&backendv1.GetArticleByIDRequest{ArticleId: "a1"})

	resp, err := h.GetArticleByID(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Article.Id != "a1" {
		t.Errorf("expected ID a1, got %s", resp.Msg.Article.Id)
	}
}

func TestGetArticleByID_EmptyID(t *testing.T) {
	h, _, _, _, _, _ := setupHandler(t)
	ctx := context.Background()

	req := connect.NewRequest(&backendv1.GetArticleByIDRequest{ArticleId: ""})

	_, err := h.GetArticleByID(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestGetArticleByID_NotFound(t *testing.T) {
	h, _, _, _, _, mockGetByID := setupHandler(t)
	ctx := context.Background()

	mockGetByID.EXPECT().
		GetArticleByID(gomock.Any(), "missing").
		Return(nil, errors.New("not found"))

	req := connect.NewRequest(&backendv1.GetArticleByIDRequest{ArticleId: "missing"})

	_, err := h.GetArticleByID(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}

// ── Phase 2 RPC tests ──

func TestCheckArticleExists_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCheckExists := mocks.NewMockCheckArticleExistsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(mockCheckExists, nil, nil, nil, nil, nil))

	mockCheckExists.EXPECT().
		CheckArticleExists(gomock.Any(), "http://example.com/article", "feed-1").
		Return(true, "article-123", nil)

	req := connect.NewRequest(&backendv1.CheckArticleExistsRequest{
		Url:    "http://example.com/article",
		FeedId: "feed-1",
	})

	resp, err := h.CheckArticleExists(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Msg.Exists {
		t.Error("expected exists to be true")
	}
	if resp.Msg.ArticleId != "article-123" {
		t.Errorf("expected article_id article-123, got %s", resp.Msg.ArticleId)
	}
}

func TestCheckArticleExists_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCheckExists := mocks.NewMockCheckArticleExistsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(mockCheckExists, nil, nil, nil, nil, nil))

	mockCheckExists.EXPECT().
		CheckArticleExists(gomock.Any(), "http://example.com/new", "feed-1").
		Return(false, "", nil)

	req := connect.NewRequest(&backendv1.CheckArticleExistsRequest{
		Url:    "http://example.com/new",
		FeedId: "feed-1",
	})

	resp, err := h.CheckArticleExists(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Exists {
		t.Error("expected exists to be false")
	}
}

func TestCheckArticleExists_MissingURL(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(mocks.NewMockCheckArticleExistsPort(gomock.NewController(t)), nil, nil, nil, nil, nil))

	req := connect.NewRequest(&backendv1.CheckArticleExistsRequest{FeedId: "feed-1"})
	_, err := h.CheckArticleExists(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestCreateArticle_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil))

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), internal_article_port.CreateArticleParams{
			Title:   "Test Article",
			URL:     "http://example.com/test",
			Content: "Hello world",
			FeedID:  "feed-1",
			UserID:  "user-1",
		}).
		Return("new-article-id", true, nil)

	req := connect.NewRequest(&backendv1.CreateArticleRequest{
		Title:   "Test Article",
		Url:     "http://example.com/test",
		Content: "Hello world",
		FeedId:  "feed-1",
		UserId:  "user-1",
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "new-article-id" {
		t.Errorf("expected article_id new-article-id, got %s", resp.Msg.ArticleId)
	}
}

func TestCreateArticle_MissingURL(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, mocks.NewMockCreateArticlePort(gomock.NewController(t)), nil, nil, nil, nil))

	req := connect.NewRequest(&backendv1.CreateArticleRequest{FeedId: "feed-1"})
	_, err := h.CreateArticle(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestSaveArticleSummary_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSave := mocks.NewMockSaveArticleSummaryPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, nil, mockSave, nil, nil, nil))

	mockSave.EXPECT().
		SaveArticleSummary(gomock.Any(), "article-1", "user-uuid-1", "This is a summary", "ja").
		Return(nil)

	req := connect.NewRequest(&backendv1.SaveArticleSummaryRequest{
		ArticleId: "article-1",
		Summary:   "This is a summary",
		Language:  "ja",
		UserId:    "user-uuid-1",
	})

	resp, err := h.SaveArticleSummary(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Msg.Success {
		t.Error("expected success to be true")
	}
}

func TestSaveArticleSummary_MissingArticleID(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, nil, mocks.NewMockSaveArticleSummaryPort(gomock.NewController(t)), nil, nil, nil))

	req := connect.NewRequest(&backendv1.SaveArticleSummaryRequest{Summary: "text", UserId: "user-1"})
	_, err := h.SaveArticleSummary(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestSaveArticleSummary_MissingUserID(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, nil, mocks.NewMockSaveArticleSummaryPort(gomock.NewController(t)), nil, nil, nil))

	req := connect.NewRequest(&backendv1.SaveArticleSummaryRequest{ArticleId: "article-1", Summary: "text"})
	_, err := h.SaveArticleSummary(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestGetArticleContent_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockContent := mocks.NewMockGetArticleContentPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, nil, nil, mockContent, nil, nil))

	mockContent.EXPECT().
		GetArticleContent(gomock.Any(), "article-1").
		Return(&internal_article_port.ArticleContent{
			ID: "article-1", Title: "Title", Content: "Body", URL: "http://example.com", UserID: "user-123",
		}, nil)

	req := connect.NewRequest(&backendv1.GetArticleContentRequest{ArticleId: "article-1"})
	resp, err := h.GetArticleContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "article-1" {
		t.Errorf("expected article_id article-1, got %s", resp.Msg.ArticleId)
	}
	if resp.Msg.Title != "Title" {
		t.Errorf("expected title Title, got %s", resp.Msg.Title)
	}
	if resp.Msg.UserId != "user-123" {
		t.Errorf("expected user_id user-123, got %s", resp.Msg.UserId)
	}
}

func TestGetArticleContent_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockContent := mocks.NewMockGetArticleContentPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, nil, nil, mockContent, nil, nil))

	mockContent.EXPECT().
		GetArticleContent(gomock.Any(), "missing").
		Return(nil, nil)

	req := connect.NewRequest(&backendv1.GetArticleContentRequest{ArticleId: "missing"})
	_, err := h.GetArticleContent(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}

func TestGetFeedID_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetFeed := mocks.NewMockGetFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, nil, nil, nil, mockGetFeed, nil))

	mockGetFeed.EXPECT().
		GetFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("feed-123", nil)

	req := connect.NewRequest(&backendv1.GetFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	resp, err := h.GetFeedID(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.FeedId != "feed-123" {
		t.Errorf("expected feed_id feed-123, got %s", resp.Msg.FeedId)
	}
}

func TestGetFeedID_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetFeed := mocks.NewMockGetFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, nil, nil, nil, mockGetFeed, nil))

	mockGetFeed.EXPECT().
		GetFeedID(gomock.Any(), "http://missing.com/feed.xml").
		Return("", errors.New("not found"))

	req := connect.NewRequest(&backendv1.GetFeedIDRequest{FeedUrl: "http://missing.com/feed.xml"})
	_, err := h.GetFeedID(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}

func TestListFeedURLs_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockListFeeds := mocks.NewMockListFeedURLsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, nil, nil, nil, nil, mockListFeeds))

	mockListFeeds.EXPECT().
		ListFeedURLs(gomock.Any(), "", 200).
		Return([]internal_feed_port.FeedURL{
			{FeedID: "f1", URL: "http://example.com/feed1.xml"},
			{FeedID: "f2", URL: "http://example.com/feed2.xml"},
		}, "f2", true, nil)

	req := connect.NewRequest(&backendv1.ListFeedURLsRequest{Limit: 200})
	resp, err := h.ListFeedURLs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Feeds) != 2 {
		t.Fatalf("expected 2 feeds, got %d", len(resp.Msg.Feeds))
	}
	if resp.Msg.Feeds[0].FeedId != "f1" {
		t.Errorf("expected first feed_id f1, got %s", resp.Msg.Feeds[0].FeedId)
	}
	if !resp.Msg.HasMore {
		t.Error("expected has_more to be true")
	}
	if resp.Msg.NextCursor != "f2" {
		t.Errorf("expected next_cursor f2, got %s", resp.Msg.NextCursor)
	}
}

// ── Summarization RPC tests ──

func TestListUnsummarizedArticles_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockList := mocks.NewMockListUnsummarizedArticlesPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithSummarizationPorts(mockList, nil))

	now := time.Now()
	expected := []*internal_article_port.UnsummarizedArticle{
		{ID: "a1", Title: "Title 1", Content: "Content 1", URL: "http://example.com/1", CreatedAt: now, UserID: "u1"},
		{ID: "a2", Title: "Title 2", Content: "Content 2", URL: "http://example.com/2", CreatedAt: now.Add(-time.Hour), UserID: "u1"},
	}

	mockList.EXPECT().
		ListUnsummarizedArticles(gomock.Any(), (*time.Time)(nil), "", 200).
		Return(expected, &now, "a2", nil)

	req := connect.NewRequest(&backendv1.ListUnsummarizedArticlesRequest{Limit: 200})
	resp, err := h.ListUnsummarizedArticles(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(resp.Msg.Articles))
	}
	if resp.Msg.Articles[0].Id != "a1" {
		t.Errorf("expected first article ID a1, got %s", resp.Msg.Articles[0].Id)
	}
	if resp.Msg.Articles[0].Url != "http://example.com/1" {
		t.Errorf("expected first article URL http://example.com/1, got %s", resp.Msg.Articles[0].Url)
	}
	if resp.Msg.NextId != "a2" {
		t.Errorf("expected next_id a2, got %s", resp.Msg.NextId)
	}
	if resp.Msg.NextCreatedAt == nil {
		t.Fatal("expected next_created_at to be set")
	}
}

func TestListUnsummarizedArticles_WithCursor(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockList := mocks.NewMockListUnsummarizedArticlesPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithSummarizationPorts(mockList, nil))

	cursorTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	mockList.EXPECT().
		ListUnsummarizedArticles(gomock.Any(), gomock.Any(), "prev-id", 100).
		Return([]*internal_article_port.UnsummarizedArticle{}, (*time.Time)(nil), "", nil)

	req := connect.NewRequest(&backendv1.ListUnsummarizedArticlesRequest{
		LastCreatedAt: timestamppb.New(cursorTime),
		LastId:        "prev-id",
		Limit:         100,
	})

	resp, err := h.ListUnsummarizedArticles(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Articles) != 0 {
		t.Fatalf("expected 0 articles, got %d", len(resp.Msg.Articles))
	}
}

func TestListUnsummarizedArticles_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockList := mocks.NewMockListUnsummarizedArticlesPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithSummarizationPorts(mockList, nil))

	mockList.EXPECT().
		ListUnsummarizedArticles(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil, "", errors.New("db error"))

	req := connect.NewRequest(&backendv1.ListUnsummarizedArticlesRequest{Limit: 200})
	_, err := h.ListUnsummarizedArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
}

func TestListUnsummarizedArticles_Unimplemented(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, nil)

	req := connect.NewRequest(&backendv1.ListUnsummarizedArticlesRequest{Limit: 200})
	_, err := h.ListUnsummarizedArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", connect.CodeOf(err))
	}
}

func TestHasUnsummarizedArticles_True(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHas := mocks.NewMockHasUnsummarizedArticlesPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithSummarizationPorts(nil, mockHas))

	mockHas.EXPECT().
		HasUnsummarizedArticles(gomock.Any()).
		Return(true, nil)

	req := connect.NewRequest(&backendv1.HasUnsummarizedArticlesRequest{})
	resp, err := h.HasUnsummarizedArticles(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Msg.HasUnsummarized {
		t.Error("expected has_unsummarized to be true")
	}
}

func TestHasUnsummarizedArticles_False(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHas := mocks.NewMockHasUnsummarizedArticlesPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithSummarizationPorts(nil, mockHas))

	mockHas.EXPECT().
		HasUnsummarizedArticles(gomock.Any()).
		Return(false, nil)

	req := connect.NewRequest(&backendv1.HasUnsummarizedArticlesRequest{})
	resp, err := h.HasUnsummarizedArticles(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.HasUnsummarized {
		t.Error("expected has_unsummarized to be false")
	}
}

func TestHasUnsummarizedArticles_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHas := mocks.NewMockHasUnsummarizedArticlesPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithSummarizationPorts(nil, mockHas))

	mockHas.EXPECT().
		HasUnsummarizedArticles(gomock.Any()).
		Return(false, errors.New("db error"))

	req := connect.NewRequest(&backendv1.HasUnsummarizedArticlesRequest{})
	_, err := h.HasUnsummarizedArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
}

func TestHasUnsummarizedArticles_Unimplemented(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, nil)

	req := connect.NewRequest(&backendv1.HasUnsummarizedArticlesRequest{})
	_, err := h.HasUnsummarizedArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", connect.CodeOf(err))
	}
}

// ── CreateArticle event publishing tests ──

func TestCreateArticle_PublishesArticleCreatedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	mockPublisher := mocks.NewMockEventPublisherPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
	)

	publishedAt := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), internal_article_port.CreateArticleParams{
			Title:       "Test Article",
			URL:         "http://example.com/test",
			Content:     "Hello world",
			FeedID:      "feed-1",
			UserID:      "user-1",
			PublishedAt: publishedAt,
		}).
		Return("new-article-id", true, nil)

	mockPublisher.EXPECT().IsEnabled().Return(true)
	mockPublisher.EXPECT().
		PublishArticleCreated(gomock.Any(), event_publisher_port.ArticleCreatedEvent{
			ArticleID:   "new-article-id",
			UserID:      "user-1",
			FeedID:      "feed-1",
			Title:       "Test Article",
			URL:         "http://example.com/test",
			Content:     "Hello world",
			PublishedAt: publishedAt,
		}).
		Return(nil)

	req := connect.NewRequest(&backendv1.CreateArticleRequest{
		Title:       "Test Article",
		Url:         "http://example.com/test",
		Content:     "Hello world",
		FeedId:      "feed-1",
		UserId:      "user-1",
		PublishedAt: timestamppb.New(publishedAt),
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "new-article-id" {
		t.Errorf("expected article_id new-article-id, got %s", resp.Msg.ArticleId)
	}
}

func TestCreateArticle_NilEventPublisher(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)

	// No WithEventPublisher — eventPublisher is nil
	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-1", true, nil)

	req := connect.NewRequest(&backendv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com",
		FeedId: "feed-1",
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "article-1" {
		t.Errorf("expected article_id article-1, got %s", resp.Msg.ArticleId)
	}
}

func TestCreateArticle_EventPublishFailureDoesNotFailRPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	mockPublisher := mocks.NewMockEventPublisherPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-2", true, nil)

	mockPublisher.EXPECT().IsEnabled().Return(true)
	mockPublisher.EXPECT().
		PublishArticleCreated(gomock.Any(), gomock.Any()).
		Return(errors.New("redis connection refused"))

	req := connect.NewRequest(&backendv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/test2",
		FeedId: "feed-1",
	})

	// RPC should succeed even though event publishing failed
	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error despite publish failure, got: %v", err)
	}
	if resp.Msg.ArticleId != "article-2" {
		t.Errorf("expected article_id article-2, got %s", resp.Msg.ArticleId)
	}
}

func TestCreateArticle_PublishesArticleUpdatedEventForUpsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	mockPublisher := mocks.NewMockEventPublisherPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
	)

	publishedAt := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("existing-article-id", false, nil)

	mockPublisher.EXPECT().IsEnabled().Return(true)
	mockPublisher.EXPECT().
		PublishArticleUpdated(gomock.Any(), event_publisher_port.ArticleUpdatedEvent{
			ArticleID:   "existing-article-id",
			UserID:      "user-1",
			FeedID:      "feed-1",
			Title:       "Updated Article",
			URL:         "http://example.com/existing",
			Content:     "Updated body",
			PublishedAt: publishedAt,
		}).
		Return(nil)

	req := connect.NewRequest(&backendv1.CreateArticleRequest{
		Title:       "Updated Article",
		Url:         "http://example.com/existing",
		Content:     "Updated body",
		FeedId:      "feed-1",
		UserId:      "user-1",
		PublishedAt: timestamppb.New(publishedAt),
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "existing-article-id" {
		t.Errorf("expected article_id existing-article-id, got %s", resp.Msg.ArticleId)
	}
}

func TestCreateArticle_EventPublisherDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	mockPublisher := mocks.NewMockEventPublisherPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-3", true, nil)

	mockPublisher.EXPECT().IsEnabled().Return(false)
	// PublishArticleCreated should NOT be called when disabled

	req := connect.NewRequest(&backendv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/test3",
		FeedId: "feed-1",
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "article-3" {
		t.Errorf("expected article_id article-3, got %s", resp.Msg.ArticleId)
	}
}

// ── GetEmptyFeedID RPC tests ──

func TestGetEmptyFeedID_Found(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGet := mocks.NewMockGetEmptyFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithBackfillPorts(mockGet))

	mockGet.EXPECT().
		GetEmptyFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("empty-feed-123", nil)

	req := connect.NewRequest(&backendv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	resp, err := h.GetEmptyFeedID(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.FeedId != "empty-feed-123" {
		t.Errorf("expected feed_id empty-feed-123, got %s", resp.Msg.FeedId)
	}
}

func TestGetEmptyFeedID_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGet := mocks.NewMockGetEmptyFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithBackfillPorts(mockGet))

	mockGet.EXPECT().
		GetEmptyFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("", nil)

	req := connect.NewRequest(&backendv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	resp, err := h.GetEmptyFeedID(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.FeedId != "" {
		t.Errorf("expected empty feed_id, got %s", resp.Msg.FeedId)
	}
}

func TestGetEmptyFeedID_EmptyFeedURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGet := mocks.NewMockGetEmptyFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithBackfillPorts(mockGet))

	req := connect.NewRequest(&backendv1.GetEmptyFeedIDRequest{FeedUrl: ""})
	_, err := h.GetEmptyFeedID(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestGetEmptyFeedID_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGet := mocks.NewMockGetEmptyFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithBackfillPorts(mockGet))

	mockGet.EXPECT().
		GetEmptyFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("", errors.New("db error"))

	req := connect.NewRequest(&backendv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	_, err := h.GetEmptyFeedID(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
}

func TestGetEmptyFeedID_Unimplemented(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, nil)

	req := connect.NewRequest(&backendv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	_, err := h.GetEmptyFeedID(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", connect.CodeOf(err))
	}
}

// ── Phase 3: ListUntaggedArticles RPC tests ──

func TestListUntaggedArticles_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockList := mocks.NewMockListUntaggedArticlesPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase3Ports(nil, nil, mockList))

	now := time.Now()
	feedID := "feed-1"
	expected := []internal_tag_port.UntaggedArticle{
		{ID: "a1", Title: "Title 1", Content: "Content 1", UserID: "u1", FeedID: &feedID, CreatedAt: now},
		{ID: "a2", Title: "Title 2", Content: "Content 2", UserID: "u1", FeedID: &feedID, CreatedAt: now.Add(-time.Hour)},
	}

	mockList.EXPECT().
		ListUntaggedArticles(gomock.Any(), (*time.Time)(nil), "", 200).
		Return(expected, &now, "a2", int32(5), nil)

	req := connect.NewRequest(&backendv1.ListUntaggedArticlesRequest{Limit: 200})
	resp, err := h.ListUntaggedArticles(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(resp.Msg.Articles))
	}
	if resp.Msg.Articles[0].Id != "a1" {
		t.Errorf("expected first article ID a1, got %s", resp.Msg.Articles[0].Id)
	}
	if resp.Msg.Articles[0].CreatedAt == nil {
		t.Error("expected created_at to be set on article")
	}
	if resp.Msg.NextId != "a2" {
		t.Errorf("expected next_id a2, got %s", resp.Msg.NextId)
	}
	if resp.Msg.NextCreatedAt == nil {
		t.Fatal("expected next_created_at to be set")
	}
	if resp.Msg.TotalCount != 5 {
		t.Errorf("expected total_count 5, got %d", resp.Msg.TotalCount)
	}
}

func TestListUntaggedArticles_WithCursor(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockList := mocks.NewMockListUntaggedArticlesPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase3Ports(nil, nil, mockList))

	cursorTime := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	mockList.EXPECT().
		ListUntaggedArticles(gomock.Any(), gomock.Any(), "prev-id", 100).
		Return([]internal_tag_port.UntaggedArticle{}, (*time.Time)(nil), "", int32(0), nil)

	req := connect.NewRequest(&backendv1.ListUntaggedArticlesRequest{
		LastCreatedAt: timestamppb.New(cursorTime),
		LastId:        "prev-id",
		Limit:         100,
	})

	resp, err := h.ListUntaggedArticles(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Articles) != 0 {
		t.Fatalf("expected 0 articles, got %d", len(resp.Msg.Articles))
	}
}

func TestListUntaggedArticles_Unimplemented(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, nil)

	req := connect.NewRequest(&backendv1.ListUntaggedArticlesRequest{Limit: 10})
	_, err := h.ListUntaggedArticles(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", connect.CodeOf(err))
	}
}

// ── BatchUpsertArticleTags: basic success without TSV ──

func TestBatchUpsertArticleTags_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBatchUpsert := mocks.NewMockBatchUpsertArticleTagsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase3Ports(nil, mockBatchUpsert, nil))

	mockBatchUpsert.EXPECT().
		BatchUpsertArticleTags(gomock.Any(), gomock.Any()).
		Return(int32(3), nil)

	req := connect.NewRequest(&backendv1.BatchUpsertArticleTagsRequest{
		Items: []*backendv1.UpsertArticleTagsRequest{
			{
				ArticleId: "art-1",
				FeedId:    "feed-1",
				Tags:      []*backendv1.TagItem{{Name: "go", Confidence: 0.9}},
			},
			{
				ArticleId: "art-2",
				FeedId:    "", // empty feed_id
				Tags:      []*backendv1.TagItem{{Name: "rust", Confidence: 0.8}},
			},
		},
	})

	resp, err := h.BatchUpsertArticleTags(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Msg.Success {
		t.Error("expected success to be true")
	}
	if resp.Msg.TotalUpserted != 3 {
		t.Errorf("expected total_upserted 3, got %d", resp.Msg.TotalUpserted)
	}
}

func TestBatchUpsertArticleTags_Unimplemented(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, nil)

	req := connect.NewRequest(&backendv1.BatchUpsertArticleTagsRequest{})
	_, err := h.BatchUpsertArticleTags(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", connect.CodeOf(err))
	}
}

// ── Knowledge Home ArticleCreated event tests ──

type stubKnowledgeEventPort struct {
	called    bool
	lastEvent domain.KnowledgeEvent
	err       error
}

func (s *stubKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) error {
	s.called = true
	s.lastEvent = event
	return s.err
}

func TestCreateArticle_AppendsKnowledgeEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{}

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("new-article-id", true, nil)

	req := connect.NewRequest(&backendv1.CreateArticleRequest{
		Title:  "Test Article",
		Url:    "http://example.com/test",
		FeedId: "feed-1",
		UserId: "00000000-0000-0000-0000-000000000001",
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "new-article-id" {
		t.Errorf("expected article_id new-article-id, got %s", resp.Msg.ArticleId)
	}

	// Knowledge event should have been appended
	if !stub.called {
		t.Fatal("expected knowledge event to be appended")
	}
	if stub.lastEvent.EventType != domain.EventArticleCreated {
		t.Errorf("expected event type ArticleCreated, got %s", stub.lastEvent.EventType)
	}
	if stub.lastEvent.AggregateID != "new-article-id" {
		t.Errorf("expected aggregate_id new-article-id, got %s", stub.lastEvent.AggregateID)
	}
}

func TestCreateArticle_NoKnowledgeEventOnUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{}

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("existing-article-id", false, nil) // created=false means upsert

	req := connect.NewRequest(&backendv1.CreateArticleRequest{
		Title:  "Updated",
		Url:    "http://example.com/existing",
		FeedId: "feed-1",
	})

	_, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Knowledge event should NOT be appended for upserts
	if stub.called {
		t.Fatal("expected knowledge event NOT to be appended for upsert")
	}
}

func TestCreateArticle_KnowledgeEventFailureDoesNotFailRPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{err: errors.New("sovereign unavailable")}

	h := NewHandler(nil, nil, nil, nil, nil, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-x", true, nil)

	req := connect.NewRequest(&backendv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/test-fail",
		FeedId: "feed-1",
	})

	// RPC should succeed even when knowledge event append fails
	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error despite knowledge event failure, got: %v", err)
	}
	if resp.Msg.ArticleId != "article-x" {
		t.Errorf("expected article_id article-x, got %s", resp.Msg.ArticleId)
	}
}

func TestClampLimit(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 200},
		{-1, 200},
		{100, 100},
		{500, 500},
		{501, 500},
		{1000, 500},
	}

	for _, tt := range tests {
		got := clampLimit(tt.input)
		if got != tt.expected {
			t.Errorf("clampLimit(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}
