package datahub_gateway

import (
	"testing"
	"time"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/orchestrator/driver/models"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestFeedSummaryFromProto(t *testing.T) {
	t.Run("nil proto returns nil", func(t *testing.T) {
		assert.Nil(t, feedSummaryFromProto(nil))
	})

	t.Run("valid proto maps summary string", func(t *testing.T) {
		pb := &datahubv1.FeedSummary{Summary: "Summary content"}
		got := feedSummaryFromProto(pb)
		assert.NotNil(t, got)
		assert.Equal(t, "Summary content", got.Summary)
	})
}

func TestFeedModelFromProto(t *testing.T) {
	t.Run("nil proto returns nil", func(t *testing.T) {
		assert.Nil(t, feedModelFromProto(nil))
	})

	t.Run("valid proto maps all 11 fields", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		articleID := "article-123"
		linkID := "link-1"
		ogURL := "https://example.com/og.png"
		pb := &datahubv1.Feed{
			Id:          "feed-1",
			Title:       "Title 1",
			Description: "Desc 1",
			WebsiteUrl:  "https://example.com",
			PubDate:     timestamppb.New(now),
			CreatedAt:   timestamppb.New(now),
			UpdatedAt:   timestamppb.New(now),
			ArticleId:   &articleID,
			IsRead:      true,
			FeedLinkId:  &linkID,
			OgImageUrl:  &ogURL,
		}

		got := feedModelFromProto(pb)
		assert.NotNil(t, got)
		assert.Equal(t, "feed-1", got.ID)
		assert.Equal(t, "Title 1", got.Title)
		assert.Equal(t, "Desc 1", got.Description)
		assert.Equal(t, "https://example.com", got.WebsiteURL)
		assert.Equal(t, now.Unix(), got.PubDate.Unix())
		assert.Equal(t, now.Unix(), got.CreatedAt.Unix())
		assert.Equal(t, now.Unix(), got.UpdatedAt.Unix())
		assert.Equal(t, &articleID, got.ArticleID)
		assert.True(t, got.IsRead)
		assert.Equal(t, &linkID, got.FeedLinkID)
		assert.Equal(t, &ogURL, got.OgImageURL)
	})
}

func TestFeedModelToRow(t *testing.T) {
	t.Run("nil model returns nil", func(t *testing.T) {
		assert.Nil(t, feedModelToRow(nil))
	})

	t.Run("valid model maps all 11 fields", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		articleID := "article-123"
		linkID := "link-1"
		ogURL := "https://example.com/og.png"
		m := &models.Feed{
			ID:          "feed-1",
			Title:       "Title 1",
			Description: "Desc 1",
			WebsiteURL:  "https://example.com",
			PubDate:     now,
			CreatedAt:   now,
			UpdatedAt:   now,
			ArticleID:   &articleID,
			IsRead:      true,
			FeedLinkID:  &linkID,
			OgImageURL:  &ogURL,
		}

		row := feedModelToRow(m)
		assert.NotNil(t, row)
		assert.Equal(t, "feed-1", row.ID)
		assert.Equal(t, "Title 1", row.Title)
		assert.Equal(t, "Desc 1", row.Description)
		assert.Equal(t, "https://example.com", row.WebsiteURL)
		assert.Equal(t, now, row.PubDate)
		assert.Equal(t, now, row.CreatedAt)
		assert.Equal(t, now, row.UpdatedAt)
		assert.Equal(t, &articleID, row.ArticleID)
		assert.True(t, row.IsRead)
		assert.Equal(t, &linkID, row.FeedLinkID)
		assert.Equal(t, &ogURL, row.OgImageURL)
	})
}

func TestFeedRowFromProto(t *testing.T) {
	t.Run("nil proto returns nil", func(t *testing.T) {
		assert.Nil(t, feedRowFromProto(nil))
	})

	t.Run("valid proto delegates through feedModelToRow", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		pb := &datahubv1.Feed{
			Id:         "feed-2",
			Title:      "Title 2",
			WebsiteUrl: "https://example.com/2",
			PubDate:    timestamppb.New(now),
		}

		row := feedRowFromProto(pb)
		assert.NotNil(t, row)
		assert.Equal(t, "feed-2", row.ID)
		assert.Equal(t, "Title 2", row.Title)
		assert.Equal(t, "https://example.com/2", row.WebsiteURL)
		assert.Equal(t, now.Unix(), row.PubDate.Unix())
	})
}

func TestFeedModelsFromProto(t *testing.T) {
	assert.Empty(t, feedModelsFromProto(nil))
	assert.Empty(t, feedModelsFromProto([]*datahubv1.Feed{}))

	pbs := []*datahubv1.Feed{
		{Id: "1", Title: "F1"},
		nil,
		{Id: "2", Title: "F2"},
	}
	models := feedModelsFromProto(pbs)
	assert.Len(t, models, 2)
	assert.Equal(t, "1", models[0].ID)
	assert.Equal(t, "2", models[1].ID)
}
