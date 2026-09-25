package datahubapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/dataplane/port/internal_article_port"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/mocks"
	"alt/shared/usecase/create_summary_version_usecase"
)

func TestSaveArticleSummary_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSave := mocks.NewMockSaveArticleSummaryPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, nil, mockSave, nil, nil, nil))

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
		WithArticleIngestionPorts(nil, nil, mockSave, nil, nil, nil))

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
		WithArticleIngestionPorts(nil, nil, mockSave, nil, nil, nil),
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
		WithArticleIngestionPorts(nil, nil, mocks.NewMockSaveArticleSummaryPort(gomock.NewController(t)), nil, nil, nil))

	req := connect.NewRequest(&datahubv1.SaveArticleSummaryRequest{Summary: "text", UserId: "user-1"})
	_, err := h.SaveArticleSummary(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
	}
}

func TestSaveArticleSummary_MissingUserID(t *testing.T) {
	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithArticleIngestionPorts(nil, nil, mocks.NewMockSaveArticleSummaryPort(gomock.NewController(t)), nil, nil, nil))

	req := connect.NewRequest(&datahubv1.SaveArticleSummaryRequest{ArticleId: "article-1", Summary: "text"})
	_, err := h.SaveArticleSummary(context.Background(), req)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
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
