package datahubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/dataplane/port/internal_article_port"
	"alt/dataplane/port/internal_feed_port"
	"alt/dataplane/port/internal_tag_port"
	"alt/dataplane/usecase/recap_articles_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/mocks"
	"alt/shared/driver/alt_db"
	"alt/shared/port/event_publisher_port"
	"alt/shared/usecase/create_summary_version_usecase"

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

	h := NewHandler(listArticles, listForward, listDeleted, getTimestamp, getByID, &fakeSystemUser{}, &fakeRecentArticles{}, nil)
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

// ── Phase 2 RPC tests ──

func TestCheckArticleExists_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCheckExists := mocks.NewMockCheckArticleExistsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(mockCheckExists, nil, nil, nil, nil, nil))

	mockCheckExists.EXPECT().
		CheckArticleExists(gomock.Any(), "http://example.com/article", "feed-1").
		Return(true, "article-123", nil)

	req := connect.NewRequest(&datahubv1.CheckArticleExistsRequest{
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(mockCheckExists, nil, nil, nil, nil, nil))

	mockCheckExists.EXPECT().
		CheckArticleExists(gomock.Any(), "http://example.com/new", "feed-1").
		Return(false, "", nil)

	req := connect.NewRequest(&datahubv1.CheckArticleExistsRequest{
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
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(mocks.NewMockCheckArticleExistsPort(gomock.NewController(t)), nil, nil, nil, nil, nil))

	req := connect.NewRequest(&datahubv1.CheckArticleExistsRequest{FeedId: "feed-1"})
	_, err := h.CheckArticleExists(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

// testTenantID is the owner every CreateArticle test writes as. It is a real
// UUID because articles.user_id is `UUID NOT NULL` and the Knowledge Home
// event is keyed by tenant — a placeholder string never reaches the database
// in production.
const testTenantID = "00000000-0000-0000-0000-000000000001"

func TestCreateArticle_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(&stubEventPublisher{}),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}))

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), internal_article_port.CreateArticleParams{
			Title:   "Test Article",
			URL:     "http://example.com/test",
			Content: "Hello world",
			FeedID:  "feed-1",
			UserID:  testTenantID,
		}).
		Return("new-article-id", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:   "Test Article",
		Url:     "http://example.com/test",
		Content: "Hello world",
		FeedId:  "feed-1",
		UserId:  testTenantID,
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
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mocks.NewMockCreateArticlePort(gomock.NewController(t)), nil, nil, nil, nil),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}))

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{FeedId: "feed-1"})
	_, err := h.CreateArticle(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

// The owner is not optional and never was: articles.user_id is UUID NOT NULL,
// so a request without a parseable one used to reach the database and come
// back as CodeInternal. It is also the tenant the Knowledge Home event is
// keyed by, which is why the check moved in front of the write rather than
// being skipped past on the event path.
func TestCreateArticle_RejectsUnusableUserID(t *testing.T) {
	tests := []struct {
		name   string
		userID string
	}{
		{name: "empty", userID: ""},
		{name: "not a uuid", userID: "user-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
				WithPhase2Ports(nil, mocks.NewMockCreateArticlePort(gomock.NewController(t)), nil, nil, nil, nil),
				WithKnowledgeEventPort(&stubKnowledgeEventPort{}))

			req := connect.NewRequest(&datahubv1.CreateArticleRequest{
				Url:    "http://example.com/test",
				FeedId: "feed-1",
				UserId: tt.userID,
			})
			_, err := h.CreateArticle(context.Background(), req)
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
			}
		})
	}
}

func TestSaveArticleSummary_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSave := mocks.NewMockSaveArticleSummaryPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, nil, mockSave, nil, nil, nil))

	mockSave.EXPECT().
		SaveArticleSummary(gomock.Any(), internal_article_port.SaveArticleSummaryParams{
			ArticleID: "article-1",
			UserID:    "user-uuid-1",
			Summary:   "This is a summary",
			Language:  "ja",
		}).
		Return(nil)

	req := connect.NewRequest(&datahubv1.SaveArticleSummaryRequest{
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

// TestSaveArticleSummary_ForwardsArticleTitle pins the field ADR-000954 Wave 3
// batch 5 added so that alt-backend could stop writing article_summaries
// directly.
//
// Without it, routing alt-backend's summarise paths through this procedure
// would have written the empty title every existing caller sends, silently
// blanking every title alt-backend had stored.
func TestSaveArticleSummary_ForwardsArticleTitle(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSave := mocks.NewMockSaveArticleSummaryPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, nil, mockSave, nil, nil, nil))

	mockSave.EXPECT().
		SaveArticleSummary(gomock.Any(), internal_article_port.SaveArticleSummaryParams{
			ArticleID:    "article-1",
			UserID:       "user-uuid-1",
			ArticleTitle: "The article",
			Summary:      "This is a summary",
		}).
		Return(nil)

	req := connect.NewRequest(&datahubv1.SaveArticleSummaryRequest{
		ArticleId:    "article-1",
		UserId:       "user-uuid-1",
		ArticleTitle: "The article",
		Summary:      "This is a summary",
	})

	if _, err := h.SaveArticleSummary(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The summary_versions row and its SummaryVersionCreated event are one write
// as far as the caller is concerned. When the append fails after the row
// lands, the version exists in a state no projection can ever reach and no
// repair path visits, so answering Success=true makes the loss permanent —
// pre-processor reads only Success and never sends the summary again.
// Unavailable rather than Internal because the caller's correct response is to
// re-send, which supersession makes safe, exactly as on the ArticleCreated
// path.
func TestSaveArticleSummary_SummaryVersionEventFailureFailsRPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSave := mocks.NewMockSaveArticleSummaryPort(ctrl)
	mockVersion := mocks.NewMockCreateSummaryVersionPort(ctrl)
	stub := &stubKnowledgeEventPort{err: errors.New("sovereign unavailable")}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, nil, mockSave, nil, nil, nil),
		WithKnowledgeVersionUsecases(
			create_summary_version_usecase.NewCreateSummaryVersionUsecase(mockVersion, stub, &fakeSummaryVersionPort{}),
			nil,
		))

	mockSave.EXPECT().SaveArticleSummary(gomock.Any(), gomock.Any()).Return(nil)
	mockVersion.EXPECT().CreateSummaryVersion(gomock.Any(), gomock.Any()).Return(nil)

	req := connect.NewRequest(&datahubv1.SaveArticleSummaryRequest{
		ArticleId: "11111111-1111-1111-1111-111111111111",
		UserId:    testTenantID,
		Summary:   "This is a summary",
	})

	resp, err := h.SaveArticleSummary(context.Background(), req)
	if err == nil {
		t.Fatalf("expected SaveArticleSummary to fail when the SummaryVersionCreated append fails, got success=%v", resp.Msg.Success)
	}
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("expected CodeUnavailable so the caller retries, got %v", connect.CodeOf(err))
	}
}

func TestSaveArticleSummary_MissingArticleID(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, nil, mocks.NewMockSaveArticleSummaryPort(gomock.NewController(t)), nil, nil, nil))

	req := connect.NewRequest(&datahubv1.SaveArticleSummaryRequest{Summary: "text", UserId: "user-1"})
	_, err := h.SaveArticleSummary(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestSaveArticleSummary_MissingUserID(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, nil, mocks.NewMockSaveArticleSummaryPort(gomock.NewController(t)), nil, nil, nil))

	req := connect.NewRequest(&datahubv1.SaveArticleSummaryRequest{ArticleId: "article-1", Summary: "text"})
	_, err := h.SaveArticleSummary(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestGetArticleContent_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockContent := mocks.NewMockGetArticleContentPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, nil, nil, mockContent, nil, nil))

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
		WithPhase2Ports(nil, nil, nil, mockContent, nil, nil))

	mockContent.EXPECT().
		GetArticleContent(gomock.Any(), "missing").
		Return(nil, nil)

	req := connect.NewRequest(&datahubv1.GetArticleContentRequest{ArticleId: "missing"})
	_, err := h.GetArticleContent(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}

func TestGetFeedID_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetFeed := mocks.NewMockGetFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, nil, nil, nil, mockGetFeed, nil))

	mockGetFeed.EXPECT().
		GetFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("feed-123", nil)

	req := connect.NewRequest(&datahubv1.GetFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, nil, nil, nil, mockGetFeed, nil))

	// internal_article_gateway wraps whatever the driver returns, and the
	// driver reports "no feed at this URL" as alt_db.ErrFeedNotFoundByURL.
	mockGetFeed.EXPECT().
		GetFeedID(gomock.Any(), "http://missing.com/feed.xml").
		Return("", fmt.Errorf("GetFeedID: %w", alt_db.ErrFeedNotFoundByURL))

	req := connect.NewRequest(&datahubv1.GetFeedIDRequest{FeedUrl: "http://missing.com/feed.xml"})
	_, err := h.GetFeedID(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connect.CodeOf(err))
	}
}

// pre-processor reads NotFound from this RPC as "the feed is not registered":
// UpsertArticles skips every article of that batch and CheckExists treats the
// URL as unknown. A pool exhaustion or a transient DB blip answered with the
// same code would therefore drop a whole batch of ingested articles and can
// misreport a registered feed as unregistered, so only the driver's absence
// sentinel may produce it.
func TestGetFeedID_RepositoryFailureIsNotNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetFeed := mocks.NewMockGetFeedIDPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, nil, nil, nil, mockGetFeed, nil))

	mockGetFeed.EXPECT().
		GetFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("", fmt.Errorf("GetFeedID: %w", errors.New("timeout: pool exhausted")))

	req := connect.NewRequest(&datahubv1.GetFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	_, err := h.GetFeedID(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal so the caller retries instead of skipping the batch, got %v", connect.CodeOf(err))
	}
}

func TestListFeedURLs_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockListFeeds := mocks.NewMockListFeedURLsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, nil, nil, nil, nil, mockListFeeds))

	mockListFeeds.EXPECT().
		ListFeedURLs(gomock.Any(), "", 200).
		Return([]internal_feed_port.FeedURL{
			{FeedID: "f1", URL: "http://example.com/feed1.xml"},
			{FeedID: "f2", URL: "http://example.com/feed2.xml"},
		}, "f2", true, nil)

	req := connect.NewRequest(&datahubv1.ListFeedURLsRequest{Limit: 200})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithSummarizationPorts(mockList, nil))

	now := time.Now()
	expected := []*internal_article_port.UnsummarizedArticle{
		{ID: "a1", Title: "Title 1", Content: "Content 1", URL: "http://example.com/1", CreatedAt: now, UserID: "u1"},
		{ID: "a2", Title: "Title 2", Content: "Content 2", URL: "http://example.com/2", CreatedAt: now.Add(-time.Hour), UserID: "u1"},
	}

	mockList.EXPECT().
		ListUnsummarizedArticles(gomock.Any(), (*time.Time)(nil), "", 200).
		Return(expected, &now, "a2", nil)

	req := connect.NewRequest(&datahubv1.ListUnsummarizedArticlesRequest{Limit: 200})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithSummarizationPorts(mockList, nil))

	cursorTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	mockList.EXPECT().
		ListUnsummarizedArticles(gomock.Any(), gomock.Any(), "prev-id", 100).
		Return([]*internal_article_port.UnsummarizedArticle{}, (*time.Time)(nil), "", nil)

	req := connect.NewRequest(&datahubv1.ListUnsummarizedArticlesRequest{
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithSummarizationPorts(mockList, nil))

	mockList.EXPECT().
		ListUnsummarizedArticles(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil, "", errors.New("db error"))

	req := connect.NewRequest(&datahubv1.ListUnsummarizedArticlesRequest{Limit: 200})
	_, err := h.ListUnsummarizedArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
}

func TestListUnsummarizedArticles_Unimplemented(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil)

	req := connect.NewRequest(&datahubv1.ListUnsummarizedArticlesRequest{Limit: 200})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithSummarizationPorts(nil, mockHas))

	mockHas.EXPECT().
		HasUnsummarizedArticles(gomock.Any()).
		Return(true, nil)

	req := connect.NewRequest(&datahubv1.HasUnsummarizedArticlesRequest{})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithSummarizationPorts(nil, mockHas))

	mockHas.EXPECT().
		HasUnsummarizedArticles(gomock.Any()).
		Return(false, nil)

	req := connect.NewRequest(&datahubv1.HasUnsummarizedArticlesRequest{})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithSummarizationPorts(nil, mockHas))

	mockHas.EXPECT().
		HasUnsummarizedArticles(gomock.Any()).
		Return(false, errors.New("db error"))

	req := connect.NewRequest(&datahubv1.HasUnsummarizedArticlesRequest{})
	_, err := h.HasUnsummarizedArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
}

func TestHasUnsummarizedArticles_Unimplemented(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil)

	req := connect.NewRequest(&datahubv1.HasUnsummarizedArticlesRequest{})
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
	// IsEnabled is read twice: once by the option, which names the
	// publisher's wiring state in the boot log, and once on the publish
	// path itself.
	mockPublisher.EXPECT().IsEnabled().Return(true).AnyTimes()

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	publishedAt := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), internal_article_port.CreateArticleParams{
			Title:       "Test Article",
			URL:         "http://example.com/test",
			Content:     "Hello world",
			FeedID:      "feed-1",
			UserID:      testTenantID,
			PublishedAt: publishedAt,
		}).
		Return("new-article-id", true, nil)

	mockPublisher.EXPECT().
		PublishArticleCreated(gomock.Any(), event_publisher_port.ArticleCreatedEvent{
			ArticleID:   "new-article-id",
			UserID:      testTenantID,
			FeedID:      "feed-1",
			Title:       "Test Article",
			URL:         "http://example.com/test",
			Content:     "Hello world",
			PublishedAt: publishedAt,
		}).
		Return(nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:       "Test Article",
		Url:         "http://example.com/test",
		Content:     "Hello world",
		FeedId:      "feed-1",
		UserId:      testTenantID,
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

// CLAUDE.md rule 8, the same refusal the knowledge event port gets one
// function below: an unwired producer must not be mistakable for a switched
// off one. mq-hub answers IsEnabled from its own configuration, so "publish
// nothing" is a state this handler can legitimately be in — reached by wiring
// a disabled publisher, never by omitting the option. Skipping a nil one
// writes the article, answers 200, and leaves summarisation and indexing with
// nothing to consume, which is the ADR-000928 failure mode exactly.
//
// This test replaces TestCreateArticle_NilEventPublisher, which pinned the
// skip.
func TestCreateArticle_PanicsWhenEventPublisherIsUnwired(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)

	// No WithEventPublisher — eventPublisher is nil
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-1", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	defer func() {
		if recover() == nil {
			t.Fatal("CreateArticle returned instead of panicking on an unwired event publisher")
		}
	}()
	_, _ = h.CreateArticle(context.Background(), req)
}

// And the disabled half of the same distinction: a publisher that is wired and
// reports itself off is a deployment choice, so the RPC succeeds and publishes
// nothing.
func TestCreateArticle_WiredButDisabledPublisherPublishesNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	publisher := &stubEventPublisher{}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(publisher),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-1", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	resp, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.ArticleId != "article-1" {
		t.Errorf("expected article_id article-1, got %s", resp.Msg.ArticleId)
	}
	if len(publisher.created) != 0 {
		t.Errorf("expected no publish while disabled, got %d", len(publisher.created))
	}
}

// stubEventPublisher stands in for mq-hub wherever a test is about something
// else. enabled is what the real gateway reads off its own client config, so
// the zero value is a wired publisher that is switched off — the state the
// handler must tell apart from a missing option.
type stubEventPublisher struct {
	enabled bool
	created []event_publisher_port.ArticleCreatedEvent
	updated []event_publisher_port.ArticleUpdatedEvent
}

func (s *stubEventPublisher) PublishArticleCreated(_ context.Context, e event_publisher_port.ArticleCreatedEvent) error {
	s.created = append(s.created, e)
	return nil
}

func (s *stubEventPublisher) PublishArticleUpdated(_ context.Context, e event_publisher_port.ArticleUpdatedEvent) error {
	s.updated = append(s.updated, e)
	return nil
}

func (s *stubEventPublisher) PublishSummarizeRequested(context.Context, event_publisher_port.SummarizeRequestedEvent) error {
	return nil
}

func (s *stubEventPublisher) PublishIndexArticle(context.Context, event_publisher_port.IndexArticleEvent) error {
	return nil
}

func (s *stubEventPublisher) IsEnabled() bool { return s.enabled }

func TestCreateArticle_EventPublishFailureDoesNotFailRPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	mockPublisher := mocks.NewMockEventPublisherPort(ctrl)
	// IsEnabled is read twice: once by the option, which names the
	// publisher's wiring state in the boot log, and once on the publish
	// path itself.
	mockPublisher.EXPECT().IsEnabled().Return(true).AnyTimes()

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-2", true, nil)

	mockPublisher.EXPECT().
		PublishArticleCreated(gomock.Any(), gomock.Any()).
		Return(errors.New("redis connection refused"))

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/test2",
		FeedId: "feed-1",
		UserId: testTenantID,
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
	// IsEnabled is read twice: once by the option, which names the
	// publisher's wiring state in the boot log, and once on the publish
	// path itself.
	mockPublisher.EXPECT().IsEnabled().Return(true).AnyTimes()

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	publishedAt := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("existing-article-id", false, nil)

	mockPublisher.EXPECT().
		PublishArticleUpdated(gomock.Any(), event_publisher_port.ArticleUpdatedEvent{
			ArticleID:   "existing-article-id",
			UserID:      testTenantID,
			FeedID:      "feed-1",
			Title:       "Updated Article",
			URL:         "http://example.com/existing",
			Content:     "Updated body",
			PublishedAt: publishedAt,
		}).
		Return(nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:       "Updated Article",
		Url:         "http://example.com/existing",
		Content:     "Updated body",
		FeedId:      "feed-1",
		UserId:      testTenantID,
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
	// IsEnabled is read twice: once by the option, which names the
	// publisher's wiring state in the boot log, and once on the publish
	// path itself.
	mockPublisher.EXPECT().IsEnabled().Return(false).AnyTimes()

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(mockPublisher),
		WithKnowledgeEventPort(&stubKnowledgeEventPort{}),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-3", true, nil)

	// PublishArticleCreated should NOT be called when disabled

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/test3",
		FeedId: "feed-1",
		UserId: testTenantID,
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBackfillPorts(mockGet))

	mockGet.EXPECT().
		GetEmptyFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("empty-feed-123", nil)

	req := connect.NewRequest(&datahubv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBackfillPorts(mockGet))

	mockGet.EXPECT().
		GetEmptyFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("", nil)

	req := connect.NewRequest(&datahubv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBackfillPorts(mockGet))

	req := connect.NewRequest(&datahubv1.GetEmptyFeedIDRequest{FeedUrl: ""})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBackfillPorts(mockGet))

	mockGet.EXPECT().
		GetEmptyFeedID(gomock.Any(), "http://example.com/feed.xml").
		Return("", errors.New("db error"))

	req := connect.NewRequest(&datahubv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
	_, err := h.GetEmptyFeedID(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connect.CodeOf(err))
	}
}

func TestGetEmptyFeedID_Unimplemented(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil)

	req := connect.NewRequest(&datahubv1.GetEmptyFeedIDRequest{FeedUrl: "http://example.com/feed.xml"})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
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

	req := connect.NewRequest(&datahubv1.ListUntaggedArticlesRequest{Limit: 200})
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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase3Ports(nil, nil, mockList))

	cursorTime := time.Date(2026, 3, 17, 12, 0, 0, 0, time.UTC)

	mockList.EXPECT().
		ListUntaggedArticles(gomock.Any(), gomock.Any(), "prev-id", 100).
		Return([]internal_tag_port.UntaggedArticle{}, (*time.Time)(nil), "", int32(0), nil)

	req := connect.NewRequest(&datahubv1.ListUntaggedArticlesRequest{
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
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil)

	req := connect.NewRequest(&datahubv1.ListUntaggedArticlesRequest{Limit: 10})
	_, err := h.ListUntaggedArticles(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", connect.CodeOf(err))
	}
}

// ── BatchUpsertArticleTags: basic success without TSV ──

func TestBatchUpsertArticleTags_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBatchUpsert := mocks.NewMockBatchUpsertArticleTagsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase3Ports(nil, mockBatchUpsert, nil))

	mockBatchUpsert.EXPECT().
		BatchUpsertArticleTags(gomock.Any(), gomock.Any()).
		Return(int32(3), nil)

	req := connect.NewRequest(&datahubv1.BatchUpsertArticleTagsRequest{
		Items: []*datahubv1.UpsertArticleTagsRequest{
			{
				ArticleId: "art-1",
				FeedId:    "feed-1",
				Tags:      []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}},
			},
			{
				ArticleId: "art-2",
				FeedId:    "", // empty feed_id
				Tags:      []*datahubv1.TagItem{{Name: "rust", Confidence: 0.8}},
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
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil)

	req := connect.NewRequest(&datahubv1.BatchUpsertArticleTagsRequest{})
	_, err := h.BatchUpsertArticleTags(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", connect.CodeOf(err))
	}
}

// ── Knowledge Home ArticleCreated event tests ──

type stubKnowledgeEventPort struct {
	called    bool
	lastEvent domain.KnowledgeEvent
	seq       int64
	err       error
}

func (s *stubKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	s.called = true
	s.lastEvent = event
	if s.err != nil {
		return 0, s.err
	}
	return s.seq, nil
}

func TestCreateArticle_AppendsKnowledgeEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(&stubEventPublisher{}),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("new-article-id", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test Article",
		Url:    "http://example.com/test",
		FeedId: "feed-1",
		UserId: testTenantID,
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

// Not every feed item carries a pubDate, and published_at is a message field:
// an omitted one arrives here as a nil Timestamp and used to be formatted into
// the payload as the Go zero time, 0001-01-01.
//
// That is not a harmless placeholder. knowledge_events is INSERT-only, so the
// value the projector reads is the value Knowledge Home keeps forever, and the
// read model ranks recency over COALESCE(published_at, generated_at) — a
// year-1 item scores as two millennia stale and never appears in a recent or
// today window again. The payload's own "unknown" is the empty string, which
// the projector folds to a NULL published_at and the ranking then replaces
// with the event's generated_at.
func TestCreateArticle_OmittedPublishedAtIsUnknownRatherThanYearOne(t *testing.T) {
	publishedAt := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		publishedAt *timestamppb.Timestamp
		want        string
	}{
		{name: "omitted", publishedAt: nil, want: ""},
		{name: "sent", publishedAt: timestamppb.New(publishedAt), want: publishedAt.Format(time.RFC3339)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockCreate := mocks.NewMockCreateArticlePort(ctrl)
			stub := &stubKnowledgeEventPort{}

			h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
				WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
				WithEventPublisher(&stubEventPublisher{}),
				WithKnowledgeEventPort(stub),
			)

			mockCreate.EXPECT().
				CreateArticle(gomock.Any(), gomock.Any()).
				Return("article-published-at", true, nil)

			req := connect.NewRequest(&datahubv1.CreateArticleRequest{
				Title:       "Test Article",
				Url:         "http://example.com/test",
				FeedId:      "feed-1",
				UserId:      testTenantID,
				PublishedAt: tt.publishedAt,
			})

			if _, err := h.CreateArticle(context.Background(), req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var payload domain.ArticleCreatedPayload
			if err := json.Unmarshal(stub.lastEvent.Payload, &payload); err != nil {
				t.Fatalf("unmarshal ArticleCreated payload: %v", err)
			}
			if payload.PublishedAt != tt.want {
				t.Errorf("ArticleCreated published_at = %q, want %q", payload.PublishedAt, tt.want)
			}
		})
	}
}

// An upsert is where the repair happens, not where it is skipped.
//
// created=false says alt-db already held the (url, user_id) row; it says
// nothing about whether sovereign ever received the article. Every retry of a
// CreateArticle whose append failed comes back on this branch, and so does
// every re-crawl of an unchanged article — so "no event on update" made the
// first miss permanent and left the Home row with the blank title
// SummaryVersionCreated gives it. The append is idempotent on dedupe_key.
func TestCreateArticle_AppendsKnowledgeEventOnUpsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(&stubEventPublisher{}),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("existing-article-id", false, nil) // created=false means upsert

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Updated",
		Url:    "http://example.com/existing",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	_, err := h.CreateArticle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !stub.called {
		t.Fatal("expected knowledge event to be appended for an upsert")
	}
	if stub.lastEvent.AggregateID != "existing-article-id" {
		t.Errorf("expected aggregate_id existing-article-id, got %s", stub.lastEvent.AggregateID)
	}
	wantDedupe := fmt.Sprintf(domain.DedupeKeyArticleCreated, "existing-article-id")
	if stub.lastEvent.DedupeKey != wantDedupe {
		t.Errorf("expected dedupe_key %s, got %s", wantDedupe, stub.lastEvent.DedupeKey)
	}
}

// A dedupe hit answers event_seq 0 with no error, and that is a success: the
// article already has its ArticleCreated. Only a real failure may fail the RPC.
func TestCreateArticle_DedupedKnowledgeEventIsNotAFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{seq: 0}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(&stubEventPublisher{}),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-dedupe", false, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Already known",
		Url:    "http://example.com/known",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	if _, err := h.CreateArticle(context.Background(), req); err != nil {
		t.Fatalf("expected a dedupe hit to succeed, got: %v", err)
	}
}

// Losing ArticleCreated is what produces the blank-title Home rows, so the
// caller has to hear about it. There is no outbox row on this path to release
// back to PENDING the way the outbox worker does — the RPC's own result is the
// acknowledgement, so it is what gets withheld. pre-processor propagates the
// error and its next crawl re-sends the article.
func TestCreateArticle_KnowledgeEventFailureFailsRPC(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)
	stub := &stubKnowledgeEventPort{err: errors.New("sovereign unavailable")}

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
		WithEventPublisher(&stubEventPublisher{}),
		WithKnowledgeEventPort(stub),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-x", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/test-fail",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	_, err := h.CreateArticle(context.Background(), req)
	if err == nil {
		t.Fatal("expected CreateArticle to fail when the ArticleCreated append fails")
	}
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("expected CodeUnavailable so the caller retries, got %v", connect.CodeOf(err))
	}
}

// CLAUDE.md rule 8: an unwired producer must be distinguishable from a
// disabled one. There is no configuration in which alt-data-hub writes
// articles without appending their ArticleCreated, so the only way to reach a
// nil port here is a composition-root mistake, and a mistake that answers 200
// is the ADR-000928 failure mode.
func TestCreateArticle_PanicsWhenKnowledgeEventPortIsUnwired(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCreate := mocks.NewMockCreateArticlePort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithPhase2Ports(nil, mockCreate, nil, nil, nil, nil),
	)

	mockCreate.EXPECT().
		CreateArticle(gomock.Any(), gomock.Any()).
		Return("article-unwired", true, nil)

	req := connect.NewRequest(&datahubv1.CreateArticleRequest{
		Title:  "Test",
		Url:    "http://example.com/unwired",
		FeedId: "feed-1",
		UserId: testTenantID,
	})

	defer func() {
		if recover() == nil {
			t.Fatal("CreateArticle returned instead of panicking on an unwired knowledge event port")
		}
	}()
	_, _ = h.CreateArticle(context.Background(), req)
}

// And the same refusal one step earlier, where it costs a boot instead of a
// request: passing the option with nothing in it is the DI mistake itself.
func TestWithKnowledgeEventPort_RefusesNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("WithKnowledgeEventPort accepted a nil port")
		}
	}()
	NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithKnowledgeEventPort(nil),
	)
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

// fakeRecapArticlesUsecase is a testify-less inline mock for the handler's
// recap article window fetch.
type fakeRecapArticlesUsecase struct {
	gotInput *domain.RecapArticlesPage
	err      error
	lastCall struct {
		page     int
		pageSize int
		from     time.Time
		to       time.Time
	}
	returnPage *domain.RecapArticlesPage
}

func (f *fakeRecapArticlesUsecase) Execute(_ context.Context, input recap_articles_usecase.Input) (*domain.RecapArticlesPage, error) {
	f.lastCall.page = input.Page
	f.lastCall.pageSize = input.PageSize
	f.lastCall.from = input.From
	f.lastCall.to = input.To
	return f.returnPage, f.err
}

func TestListRecapArticles_SuccessMapsDomainToProto(t *testing.T) {
	articleID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	title := "Test Article"
	sourceURL := "https://example.com/a"
	langHint := "en"
	publishedAt := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	uc := &fakeRecapArticlesUsecase{
		returnPage: &domain.RecapArticlesPage{
			Total:    1,
			Page:     1,
			PageSize: 500,
			HasMore:  false,
			Articles: []domain.RecapArticle{{
				ID:          articleID,
				Title:       &title,
				FullText:    "Body text here.",
				SourceURL:   &sourceURL,
				LangHint:    &langHint,
				PublishedAt: &publishedAt,
			}},
		},
	}

	h, _, _, _, _, _ := setupHandler(t)
	WithRecapArticlesUsecase(uc)(h)

	pageReq := int32(2)
	pageSizeReq := int32(100)
	req := connect.NewRequest(&datahubv1.ListRecapArticlesRequest{
		From:     "2026-04-14T00:00:00Z",
		To:       "2026-04-15T00:00:00Z",
		Page:     &pageReq,
		PageSize: &pageSizeReq,
	})

	resp, err := h.ListRecapArticles(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Msg.Total)
	}
	if resp.Msg.Range.From != "2026-04-14T00:00:00Z" {
		t.Errorf("Range.From = %q", resp.Msg.Range.From)
	}
	if len(resp.Msg.Articles) != 1 {
		t.Fatalf("Articles len = %d, want 1", len(resp.Msg.Articles))
	}
	got := resp.Msg.Articles[0]
	if got.ArticleId != articleID.String() {
		t.Errorf("ArticleId = %q", got.ArticleId)
	}
	if got.Title == nil || *got.Title != title {
		t.Errorf("Title = %v", got.Title)
	}
	if got.Fulltext != "Body text here." {
		t.Errorf("Fulltext = %q", got.Fulltext)
	}
	if got.PublishedAt == nil || *got.PublishedAt != "2026-04-15T12:00:00Z" {
		t.Errorf("PublishedAt = %v", got.PublishedAt)
	}
	if uc.lastCall.page != 2 || uc.lastCall.pageSize != 100 {
		t.Errorf("usecase got page=%d size=%d", uc.lastCall.page, uc.lastCall.pageSize)
	}
}

func TestListRecapArticles_InvalidFromMissing(t *testing.T) {
	uc := &fakeRecapArticlesUsecase{}
	h, _, _, _, _, _ := setupHandler(t)
	WithRecapArticlesUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListRecapArticlesRequest{To: "2026-04-15T00:00:00Z"})
	_, err := h.ListRecapArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr, ok := err.(*connect.Error)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListRecapArticles_InvalidFromNotRFC3339(t *testing.T) {
	uc := &fakeRecapArticlesUsecase{}
	h, _, _, _, _, _ := setupHandler(t)
	WithRecapArticlesUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListRecapArticlesRequest{From: "yesterday", To: "2026-04-15T00:00:00Z"})
	_, err := h.ListRecapArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr, ok := err.(*connect.Error)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListRecapArticles_UsecaseErrorMapsToInvalidArgument(t *testing.T) {
	uc := &fakeRecapArticlesUsecase{err: errors.New("page_size must be <= 2000")}
	h, _, _, _, _, _ := setupHandler(t)
	WithRecapArticlesUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListRecapArticlesRequest{
		From: "2026-04-14T00:00:00Z",
		To:   "2026-04-15T00:00:00Z",
	})
	_, err := h.ListRecapArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr, ok := err.(*connect.Error)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListRecapArticles_NotConfiguredReturnsUnimplemented(t *testing.T) {
	h, _, _, _, _, _ := setupHandler(t)
	// No WithRecapArticlesUsecase call.

	req := connect.NewRequest(&datahubv1.ListRecapArticlesRequest{
		From: "2026-04-14T00:00:00Z",
		To:   "2026-04-15T00:00:00Z",
	})
	_, err := h.ListRecapArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr, ok := err.(*connect.Error)
	if !ok || connErr.Code() != connect.CodeUnimplemented {
		t.Errorf("expected Unimplemented, got %v", err)
	}
}

// ── BatchGetTagsByArticleIDs ──

func TestBatchGetTagsByArticleIDs_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPort := mocks.NewMockBatchGetTagsByArticleIDsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBatchGetTagsPort(mockPort))

	updatedAt := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	mockPort.EXPECT().
		BatchGetTagsByArticleIDs(gomock.Any(), []string{"a1", "a2"}).
		Return([]internal_tag_port.ArticleTagsByID{
			{
				ArticleID: "a1",
				Tags: []internal_tag_port.ArticleTagEntry{
					{TagName: "go", Confidence: 0.9, UpdatedAt: updatedAt},
					{TagName: "rust", Confidence: 0.7, UpdatedAt: updatedAt},
				},
			},
			{
				ArticleID: "a2",
				Tags: []internal_tag_port.ArticleTagEntry{
					{TagName: "python", Confidence: 0.6, UpdatedAt: updatedAt},
				},
			},
		}, nil)

	req := connect.NewRequest(&datahubv1.BatchGetTagsByArticleIDsRequest{
		ArticleIds: []string{"a1", "a2"},
	})
	resp, err := h.BatchGetTagsByArticleIDs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(resp.Msg.Items); got != 2 {
		t.Fatalf("expected 2 items, got %d", got)
	}
	if resp.Msg.Items[0].ArticleId != "a1" {
		t.Errorf("expected first article_id=a1, got %s", resp.Msg.Items[0].ArticleId)
	}
	if got := len(resp.Msg.Items[0].Tags); got != 2 {
		t.Errorf("expected 2 tags on a1, got %d", got)
	}
	if resp.Msg.Items[0].Tags[0].TagName != "go" || resp.Msg.Items[0].Tags[0].Confidence < 0.89 {
		t.Errorf("unexpected first tag on a1: %+v", resp.Msg.Items[0].Tags[0])
	}
	if resp.Msg.Items[0].Tags[0].UpdatedAt == nil || !resp.Msg.Items[0].Tags[0].UpdatedAt.AsTime().Equal(updatedAt) {
		t.Errorf("expected updated_at passthrough, got %+v", resp.Msg.Items[0].Tags[0].UpdatedAt)
	}
}

func TestBatchGetTagsByArticleIDs_EmptyRequestReturnsEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPort := mocks.NewMockBatchGetTagsByArticleIDsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBatchGetTagsPort(mockPort))

	// Port should not be called for empty input.
	req := connect.NewRequest(&datahubv1.BatchGetTagsByArticleIDsRequest{ArticleIds: nil})
	resp, err := h.BatchGetTagsByArticleIDs(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Items) != 0 {
		t.Errorf("expected empty items, got %d", len(resp.Msg.Items))
	}
}

func TestBatchGetTagsByArticleIDs_RejectsOverCap(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPort := mocks.NewMockBatchGetTagsByArticleIDsPort(ctrl)
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBatchGetTagsPort(mockPort))

	ids := make([]string, maxBatchGetTagsArticleIDs+1)
	for i := range ids {
		ids[i] = "id"
	}
	req := connect.NewRequest(&datahubv1.BatchGetTagsByArticleIDsRequest{ArticleIds: ids})
	_, err := h.BatchGetTagsByArticleIDs(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", code)
	}
}

func TestBatchGetTagsByArticleIDs_PortErrorMapsToInternal(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockPort := mocks.NewMockBatchGetTagsByArticleIDsPort(ctrl)
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithBatchGetTagsPort(mockPort))

	mockPort.EXPECT().
		BatchGetTagsByArticleIDs(gomock.Any(), []string{"a1"}).
		Return(nil, errors.New("boom"))

	req := connect.NewRequest(&datahubv1.BatchGetTagsByArticleIDsRequest{ArticleIds: []string{"a1"}})
	_, err := h.BatchGetTagsByArticleIDs(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if code := connect.CodeOf(err); code != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", code)
	}
}

func TestBatchGetTagsByArticleIDs_Unimplemented(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil)
	req := connect.NewRequest(&datahubv1.BatchGetTagsByArticleIDsRequest{ArticleIds: []string{"a1"}})
	_, err := h.BatchGetTagsByArticleIDs(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	if code := connect.CodeOf(err); code != connect.CodeUnimplemented {
		t.Errorf("expected CodeUnimplemented, got %v", code)
	}
}
