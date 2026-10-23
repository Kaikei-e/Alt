package datahubapi

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"go.uber.org/mock/gomock"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/mocks"
)

// ── BatchUpsertArticleTags: basic success without TSV ──

func TestBatchUpsertArticleTags_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBatchUpsert := mocks.NewMockBatchUpsertArticleTagsPort(ctrl)

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithTagCatalogPorts(nil, mockBatchUpsert, nil))

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

	h := NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, nil,
		WithTagCatalogPorts(mockUpsert, nil, nil))

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
