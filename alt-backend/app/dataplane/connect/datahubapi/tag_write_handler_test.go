package datahubapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"alt/dataplane/port/internal_article_port"
	"alt/dataplane/usecase/create_tag_set_version_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/mocks"
)

// ── BatchUpsertArticleTags: basic success without TSV ──

func TestBatchUpsertArticleTags_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBatchUpsert := mocks.NewMockBatchUpsertArticleTagsPort(ctrl)
	tagSetPort := &fakeTagSetVersionPort{}
	eventPort := &stubKnowledgeEventPort{}
	uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithTagCatalogPorts(nil, mockBatchUpsert, nil),
		WithKnowledgeVersionUsecases(nil, uc))

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

func TestUpsertArticleTags_ValidationAndSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockUpsert := mocks.NewMockUpsertArticleTagsPort(ctrl)
	tagSetPort := &fakeTagSetVersionPort{}
	eventPort := &stubKnowledgeEventPort{}
	uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithTagCatalogPorts(mockUpsert, nil, nil),
		WithKnowledgeVersionUsecases(nil, uc))

	t.Run("missing article_id returns CodeInvalidArgument", func(t *testing.T) {
		_, err := h.UpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.UpsertArticleTagsRequest{}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("expected CodeInvalidArgument, got %v", connect.CodeOf(err))
		}
	})

	t.Run("valid request upserts tags and returns count", func(t *testing.T) {
		mockUpsert.EXPECT().
			UpsertArticleTags(gomock.Any(), "art-1", "feed-1", gomock.Any()).
			Return(int32(2), nil)

		resp, err := h.UpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.UpsertArticleTagsRequest{
			ArticleId: "art-1",
			FeedId:    "feed-1",
			Tags: []*datahubv1.TagItem{
				{Name: "golang", Confidence: 0.95},
				{Name: "connectrpc", Confidence: 0.85},
			},
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.Msg.GetSuccess() || resp.Msg.GetUpsertedCount() != 2 {
			t.Fatalf("unexpected response: %+v", resp.Msg)
		}
	})
}

func TestUpsertArticleTags_OwnerResolutionAndErrors(t *testing.T) {
	const articleID = "11111111-1111-1111-1111-111111111111"

	t.Run("soft-deleted article (ErrNoRows) skips version creation and succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockUpsert := mocks.NewMockUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)
		tagSetPort := &fakeTagSetVersionPort{}
		eventPort := &stubKnowledgeEventPort{}
		uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(mockUpsert, nil, nil),
			WithKnowledgeVersionUsecases(nil, uc),
		)

		mockUpsert.EXPECT().
			UpsertArticleTags(gomock.Any(), articleID, "feed-1", gomock.Any()).
			Return(int32(1), nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), articleID).
			Return(nil, fmt.Errorf("GetArticleByID: %w", pgx.ErrNoRows))

		resp, err := h.UpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.UpsertArticleTagsRequest{
			ArticleId: articleID,
			FeedId:    "feed-1",
			Tags:      []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}},
		}))
		require.NoError(t, err)
		assert.True(t, resp.Msg.GetSuccess())
		assert.Empty(t, tagSetPort.created, "Execute must not be called for a soft-deleted article")
		assert.False(t, eventPort.called, "no knowledge event may be appended for a soft-deleted article")
	})

	t.Run("transient article lookup error fails RPC with CodeUnavailable and never calls Execute", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockUpsert := mocks.NewMockUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)
		tagSetPort := &fakeTagSetVersionPort{}
		eventPort := &stubKnowledgeEventPort{}
		uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(mockUpsert, nil, nil),
			WithKnowledgeVersionUsecases(nil, uc),
		)

		mockUpsert.EXPECT().
			UpsertArticleTags(gomock.Any(), articleID, "feed-1", gomock.Any()).
			Return(int32(1), nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), articleID).
			Return(nil, errors.New("timeout: pool exhausted"))

		_, err := h.UpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.UpsertArticleTagsRequest{
			ArticleId: articleID,
			FeedId:    "feed-1",
			Tags:      []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
		assert.Empty(t, tagSetPort.created, "Execute must not be called when article owner lookup fails")
		assert.False(t, eventPort.called)
	})

	t.Run("unparseable article UserID skips Execute without writing nil tenant and logs at ERROR", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockUpsert := mocks.NewMockUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)
		tagSetPort := &fakeTagSetVersionPort{}
		eventPort := &stubKnowledgeEventPort{}
		uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logBuf, nil))

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, logger,
			WithTagCatalogPorts(mockUpsert, nil, nil),
			WithKnowledgeVersionUsecases(nil, uc),
		)

		mockUpsert.EXPECT().
			UpsertArticleTags(gomock.Any(), articleID, "feed-1", gomock.Any()).
			Return(int32(1), nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), articleID).
			Return(&internal_article_port.ArticleWithTags{ID: articleID, UserID: "not-a-uuid"}, nil)

		_, err := h.UpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.UpsertArticleTagsRequest{
			ArticleId: articleID,
			FeedId:    "feed-1",
			Tags:      []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}},
		}))
		require.NoError(t, err)
		assert.Empty(t, tagSetPort.created, "Execute must not be called with uuid.Nil UserID")
		assert.False(t, eventPort.called)
		assert.Contains(t, logBuf.String(), "level=ERROR", "unparseable article owner must log at ERROR")
		assert.NotContains(t, logBuf.String(), "level=WARN", "unparseable article owner must not log at WARN")
	})

	t.Run("Execute failure returns CodeUnavailable so caller retries", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockUpsert := mocks.NewMockUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)
		tagSetPort := &fakeTagSetVersionPort{}
		eventPort := &stubKnowledgeEventPort{err: errors.New("sovereign unavailable")}
		uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(mockUpsert, nil, nil),
			WithKnowledgeVersionUsecases(nil, uc),
		)

		mockUpsert.EXPECT().
			UpsertArticleTags(gomock.Any(), articleID, "feed-1", gomock.Any()).
			Return(int32(1), nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), articleID).
			Return(&internal_article_port.ArticleWithTags{ID: articleID, UserID: testTenantID}, nil)

		_, err := h.UpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.UpsertArticleTagsRequest{
			ArticleId: articleID,
			FeedId:    "feed-1",
			Tags:      []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	})

	t.Run("permanent validation error from Execute maps to CodeInvalidArgument", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockUpsert := mocks.NewMockUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)
		tagSetPort := &fakeTagSetVersionPort{}
		eventPort := &stubKnowledgeEventPort{err: create_tag_set_version_usecase.ErrMissingUserID}
		uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(mockUpsert, nil, nil),
			WithKnowledgeVersionUsecases(nil, uc),
		)

		mockUpsert.EXPECT().
			UpsertArticleTags(gomock.Any(), articleID, "feed-1", gomock.Any()).
			Return(int32(1), nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), articleID).
			Return(&internal_article_port.ArticleWithTags{ID: articleID, UserID: testTenantID}, nil)

		_, err := h.UpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.UpsertArticleTagsRequest{
			ArticleId: articleID,
			FeedId:    "feed-1",
			Tags:      []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("panics when CreateTagSetVersionUsecase is unwired", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockUpsert := mocks.NewMockUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(mockUpsert, nil, nil),
		)

		mockUpsert.EXPECT().
			UpsertArticleTags(gomock.Any(), articleID, "feed-1", gomock.Any()).
			Return(int32(1), nil)

		assert.Panics(t, func() {
			_, _ = h.UpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.UpsertArticleTagsRequest{
				ArticleId: articleID,
				FeedId:    "feed-1",
				Tags:      []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}},
			}))
		})
	})

	t.Run("panics when GetArticleByIDPort is unwired while version usecase is wired", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockUpsert := mocks.NewMockUpsertArticleTagsPort(ctrl)
		tagSetPort := &fakeTagSetVersionPort{}
		eventPort := &stubKnowledgeEventPort{}
		uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

		h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(mockUpsert, nil, nil),
			WithKnowledgeVersionUsecases(nil, uc),
		)

		mockUpsert.EXPECT().
			UpsertArticleTags(gomock.Any(), articleID, "feed-1", gomock.Any()).
			Return(int32(1), nil)

		assert.Panics(t, func() {
			_, _ = h.UpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.UpsertArticleTagsRequest{
				ArticleId: articleID,
				FeedId:    "feed-1",
				Tags:      []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}},
			}))
		})
		assert.Empty(t, tagSetPort.created)
	})
}

func TestBatchUpsertArticleTags_OwnerResolutionAndErrors(t *testing.T) {
	const (
		article1 = "11111111-1111-1111-1111-111111111111"
		article2 = "22222222-2222-2222-2222-222222222222"
	)

	t.Run("soft-deleted article in batch is skipped while remaining items record versions", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockBatchUpsert := mocks.NewMockBatchUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)
		tagSetPort := &fakeTagSetVersionPort{}
		eventPort := &stubKnowledgeEventPort{}
		uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(nil, mockBatchUpsert, nil),
			WithKnowledgeVersionUsecases(nil, uc),
		)

		mockBatchUpsert.EXPECT().
			BatchUpsertArticleTags(gomock.Any(), gomock.Any()).
			Return(int32(2), nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), article1).
			Return(nil, fmt.Errorf("GetArticleByID: %w", pgx.ErrNoRows))
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), article2).
			Return(&internal_article_port.ArticleWithTags{ID: article2, UserID: testTenantID}, nil)

		resp, err := h.BatchUpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.BatchUpsertArticleTagsRequest{
			Items: []*datahubv1.UpsertArticleTagsRequest{
				{ArticleId: article1, FeedId: "feed-1", Tags: []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}}},
				{ArticleId: article2, FeedId: "feed-1", Tags: []*datahubv1.TagItem{{Name: "rust", Confidence: 0.8}}},
			},
		}))
		require.NoError(t, err)
		assert.True(t, resp.Msg.GetSuccess())
		require.Len(t, tagSetPort.created, 1, "only the non-deleted article may record a version")
		assert.Equal(t, uuid.MustParse(article2), tagSetPort.created[0].ArticleID)
		assert.Equal(t, uuid.MustParse(testTenantID), tagSetPort.created[0].UserID)
	})

	t.Run("transient lookup error in batch returns CodeUnavailable without calling Execute", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockBatchUpsert := mocks.NewMockBatchUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)
		tagSetPort := &fakeTagSetVersionPort{}
		eventPort := &stubKnowledgeEventPort{}
		uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(nil, mockBatchUpsert, nil),
			WithKnowledgeVersionUsecases(nil, uc),
		)

		mockBatchUpsert.EXPECT().
			BatchUpsertArticleTags(gomock.Any(), gomock.Any()).
			Return(int32(1), nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), article1).
			Return(nil, errors.New("connection reset by peer"))

		_, err := h.BatchUpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.BatchUpsertArticleTagsRequest{
			Items: []*datahubv1.UpsertArticleTagsRequest{
				{ArticleId: article1, FeedId: "feed-1", Tags: []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}}},
			},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
		assert.Empty(t, tagSetPort.created, "Execute must not be called when article lookup fails")
		assert.False(t, eventPort.called)
	})

	t.Run("Execute failure in batch returns CodeUnavailable", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockBatchUpsert := mocks.NewMockBatchUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)
		tagSetPort := &fakeTagSetVersionPort{}
		eventPort := &stubKnowledgeEventPort{err: errors.New("sovereign unavailable")}
		uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(nil, mockBatchUpsert, nil),
			WithKnowledgeVersionUsecases(nil, uc),
		)

		mockBatchUpsert.EXPECT().
			BatchUpsertArticleTags(gomock.Any(), gomock.Any()).
			Return(int32(1), nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), article1).
			Return(&internal_article_port.ArticleWithTags{ID: article1, UserID: testTenantID}, nil)

		_, err := h.BatchUpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.BatchUpsertArticleTagsRequest{
			Items: []*datahubv1.UpsertArticleTagsRequest{
				{ArticleId: article1, FeedId: "feed-1", Tags: []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}}},
			},
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	})

	t.Run("whole-batch retry after mid-batch failure yields exactly one version per article", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockBatchUpsert := mocks.NewMockBatchUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)
		tagSetPort := &fakeTagSetVersionPort{}
		eventPort := &failOnSecondCallEventPort{}
		uc := create_tag_set_version_usecase.NewCreateTagSetVersionUsecase(tagSetPort, eventPort, tagSetPort)

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(nil, mockBatchUpsert, nil),
			WithKnowledgeVersionUsecases(nil, uc),
		)

		batchReq := &datahubv1.BatchUpsertArticleTagsRequest{
			Items: []*datahubv1.UpsertArticleTagsRequest{
				{ArticleId: article1, FeedId: "feed-1", Tags: []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}}},
				{ArticleId: article2, FeedId: "feed-1", Tags: []*datahubv1.TagItem{{Name: "rust", Confidence: 0.8}}},
			},
		}

		// First attempt: article1 succeeds, article2 event append fails -> Unavailable.
		mockBatchUpsert.EXPECT().
			BatchUpsertArticleTags(gomock.Any(), gomock.Any()).
			Return(int32(2), nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), article1).
			Return(&internal_article_port.ArticleWithTags{ID: article1, UserID: testTenantID}, nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), article2).
			Return(&internal_article_port.ArticleWithTags{ID: article2, UserID: testTenantID}, nil)

		_, err := h.BatchUpsertArticleTags(context.Background(), connect.NewRequest(batchReq))
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))

		// Second attempt (whole-batch retry): both items succeed; article1 is a replay no-op.
		mockBatchUpsert.EXPECT().
			BatchUpsertArticleTags(gomock.Any(), gomock.Any()).
			Return(int32(2), nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), article1).
			Return(&internal_article_port.ArticleWithTags{ID: article1, UserID: testTenantID}, nil)
		mockGetByID.EXPECT().
			GetArticleByID(gomock.Any(), article2).
			Return(&internal_article_port.ArticleWithTags{ID: article2, UserID: testTenantID}, nil)

		resp, err := h.BatchUpsertArticleTags(context.Background(), connect.NewRequest(batchReq))
		require.NoError(t, err)
		assert.True(t, resp.Msg.GetSuccess())
		require.Len(t, tagSetPort.created, 2, "whole-batch retry must yield exactly one version per article")
		assert.Equal(t, uuid.MustParse(article1), tagSetPort.created[0].ArticleID)
		assert.Equal(t, uuid.MustParse(article2), tagSetPort.created[1].ArticleID)
		require.Len(t, eventPort.appended, 2, "whole-batch retry must yield exactly one TagSetVersionCreated per article")
	})

	t.Run("panics when CreateTagSetVersionUsecase is unwired in batch", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockBatchUpsert := mocks.NewMockBatchUpsertArticleTagsPort(ctrl)
		mockGetByID := mocks.NewMockGetArticleByIDPort(ctrl)

		h := NewHandler(nil, nil, nil, nil, mockGetByID, &fakeSystemUser{}, &fakeRecentArticles{}, testLogger(),
			WithTagCatalogPorts(nil, mockBatchUpsert, nil),
		)

		mockBatchUpsert.EXPECT().
			BatchUpsertArticleTags(gomock.Any(), gomock.Any()).
			Return(int32(1), nil)

		assert.Panics(t, func() {
			_, _ = h.BatchUpsertArticleTags(context.Background(), connect.NewRequest(&datahubv1.BatchUpsertArticleTagsRequest{
				Items: []*datahubv1.UpsertArticleTagsRequest{
					{ArticleId: article1, FeedId: "feed-1", Tags: []*datahubv1.TagItem{{Name: "go", Confidence: 0.9}}},
				},
			}))
		})
	})
}

type failOnSecondCallEventPort struct {
	calls    int
	appended []string
}

func (f *failOnSecondCallEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	f.calls++
	if f.calls == 2 {
		return 0, errors.New("sovereign unavailable")
	}
	f.appended = append(f.appended, event.DedupeKey)
	return int64(len(f.appended)), nil
}
