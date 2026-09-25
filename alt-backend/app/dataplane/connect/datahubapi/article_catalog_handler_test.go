package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/dataplane/port/internal_article_port"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/mocks"
)

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

	req := connect.NewRequest(&datahubv1.ListArticlesWithTagsRequest{
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

func TestListArticlesWithTags_PropagatesLanguage(t *testing.T) {
	h, mockList, _, _, _, _ := setupHandler(t)
	ctx := context.Background()

	now := time.Now()
	mockList.EXPECT().
		ListArticlesWithTags(gomock.Any(), (*time.Time)(nil), "", 100).
		Return([]*internal_article_port.ArticleWithTags{
			{ID: "a1", Title: "JP", Content: "c", Tags: []string{}, CreatedAt: now, UserID: "u", Language: "ja"},
			{ID: "a2", Title: "EN", Content: "c", Tags: []string{}, CreatedAt: now, UserID: "u", Language: "en"},
			{ID: "a3", Title: "UNK", Content: "c", Tags: []string{}, CreatedAt: now, UserID: "u"},
		}, &now, "a3", nil)

	resp, err := h.ListArticlesWithTags(ctx, connect.NewRequest(&datahubv1.ListArticlesWithTagsRequest{Limit: 100}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Articles[0].Language != "ja" {
		t.Errorf("expected Language ja, got %q", resp.Msg.Articles[0].Language)
	}
	if resp.Msg.Articles[1].Language != "en" {
		t.Errorf("expected Language en, got %q", resp.Msg.Articles[1].Language)
	}
	if resp.Msg.Articles[2].Language != "" {
		t.Errorf("expected Language empty default, got %q", resp.Msg.Articles[2].Language)
	}
}

func TestListArticlesWithTags_WithCursor(t *testing.T) {
	h, mockList, _, _, _, _ := setupHandler(t)
	ctx := context.Background()

	cursorTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	mockList.EXPECT().
		ListArticlesWithTags(gomock.Any(), gomock.Any(), "prev-id", 100).
		Return([]*internal_article_port.ArticleWithTags{}, (*time.Time)(nil), "", nil)

	req := connect.NewRequest(&datahubv1.ListArticlesWithTagsRequest{
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

	req := connect.NewRequest(&datahubv1.ListArticlesWithTagsRequest{Limit: 200})

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

	req := connect.NewRequest(&datahubv1.ListArticlesWithTagsForwardRequest{
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

	req := connect.NewRequest(&datahubv1.ListDeletedArticlesRequest{Limit: 200})

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

	req := connect.NewRequest(&datahubv1.GetLatestArticleTimestampRequest{})

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

	req := connect.NewRequest(&datahubv1.GetLatestArticleTimestampRequest{})

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

	req := connect.NewRequest(&datahubv1.GetArticleByIDRequest{ArticleId: "a1"})

	resp, err := h.GetArticleByID(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Article.Id != "a1" {
		t.Errorf("expected ID a1, got %s", resp.Msg.Article.Id)
	}
}

// TestGetArticleByID_PublishedAt pins the mapping of the nullable
// articles.published_at column onto the wire. search-indexer builds every
// document's date filter from this RPC, so a present timestamp must be
// forwarded and an absent one must stay unset rather than defaulting to
// created_at on the provider side.
func TestGetArticleByID_PublishedAt(t *testing.T) {
	createdAt := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	publishedAt := time.Date(2026, 3, 20, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name        string
		publishedAt *time.Time
		wantSet     bool
	}{
		{name: "published_at present", publishedAt: &publishedAt, wantSet: true},
		{name: "published_at is NULL", publishedAt: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _, _, _, mockGetByID := setupHandler(t)
			ctx := context.Background()

			mockGetByID.EXPECT().
				GetArticleByID(gomock.Any(), "a1").
				Return(&internal_article_port.ArticleWithTags{
					ID:          "a1",
					Title:       "Test",
					CreatedAt:   createdAt,
					UserID:      "u1",
					PublishedAt: tt.publishedAt,
				}, nil)

			resp, err := h.GetArticleByID(ctx, connect.NewRequest(&datahubv1.GetArticleByIDRequest{ArticleId: "a1"}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !tt.wantSet {
				if resp.Msg.Article.PublishedAt != nil {
					t.Fatalf("expected published_at to stay unset, got %v", resp.Msg.Article.PublishedAt.AsTime())
				}
				return
			}
			if resp.Msg.Article.PublishedAt == nil {
				t.Fatal("expected published_at to be set")
			}
			if got := resp.Msg.Article.PublishedAt.AsTime(); !got.Equal(publishedAt) {
				t.Errorf("published_at = %v, want %v", got, publishedAt)
			}
		})
	}
}

func TestGetArticleByID_EmptyID(t *testing.T) {
	h, _, _, _, _, _ := setupHandler(t)
	ctx := context.Background()

	req := connect.NewRequest(&datahubv1.GetArticleByIDRequest{ArticleId: ""})

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

	// internal_article_gateway wraps whatever the driver returns, and the
	// driver reports an absent (or soft-deleted) row as pgx.ErrNoRows.
	mockGetByID.EXPECT().
		GetArticleByID(gomock.Any(), "missing").
		Return(nil, fmt.Errorf("GetArticleByID: %w", pgx.ErrNoRows))

	req := connect.NewRequest(&datahubv1.GetArticleByIDRequest{ArticleId: "missing"})

	_, err := h.GetArticleByID(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}

// search-indexer skips and ACKs a GetArticleByID that answers NotFound, on the
// reading that the row is gone. A pool exhaustion or a transient DB blip that
// also answered NotFound would therefore drop the article from the index
// permanently, with no retry and no DLQ, so only the absence sentinel may
// produce that code.
func TestGetArticleByID_RepositoryFailureIsNotNotFound(t *testing.T) {
	h, _, _, _, _, mockGetByID := setupHandler(t)
	ctx := context.Background()

	mockGetByID.EXPECT().
		GetArticleByID(gomock.Any(), "a1").
		Return(nil, fmt.Errorf("GetArticleByID: %w", errors.New("timeout: pool exhausted")))

	req := connect.NewRequest(&datahubv1.GetArticleByIDRequest{ArticleId: "a1"})

	_, err := h.GetArticleByID(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal so the caller retries, got %v", connect.CodeOf(err))
	}
}

func TestGetArticleContent_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockContent := mocks.NewMockGetArticleContentPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, nil, nil, mockContent, nil, nil))

	mockContent.EXPECT().
		GetArticleContent(gomock.Any(), "article-1").
		Return(&internal_article_port.ArticleContent{
			ID: "article-1", Title: "Title", Content: "Body", URL: "http://example.com", UserID: "user-123",
		}, nil)

	req := connect.NewRequest(&datahubv1.GetArticleContentRequest{ArticleId: "article-1"})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, nil, nil, mockContent, nil, nil))

	mockContent.EXPECT().
		GetArticleContent(gomock.Any(), "missing").
		Return(nil, nil)

	req := connect.NewRequest(&datahubv1.GetArticleContentRequest{ArticleId: "missing"})
	_, err := h.GetArticleContent(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}
