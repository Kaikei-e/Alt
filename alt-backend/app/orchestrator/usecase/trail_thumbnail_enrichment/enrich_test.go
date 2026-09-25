package trail_thumbnail_enrichment

import (
	"context"
	"errors"
	"testing"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockThumbnailPort struct {
	urls map[string]string
	err  error
}

func (m *mockThumbnailPort) GetOgImageURLsByArticleIDs(_ context.Context, _ []string) (map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.urls, nil
}

func TestArticleIDFromItemKey(t *testing.T) {
	validUUID := uuid.New().String()

	tests := []struct {
		name    string
		itemKey string
		wantID  string
		wantOk  bool
	}{
		{name: "valid article item key", itemKey: "article:" + validUUID, wantID: validUUID, wantOk: true},
		{name: "non-article prefix", itemKey: "feed:" + validUUID, wantID: "", wantOk: false},
		{name: "empty prefix", itemKey: validUUID, wantID: "", wantOk: false},
		{name: "malformed uuid", itemKey: "article:invalid-uuid", wantID: "", wantOk: false},
		{name: "empty string", itemKey: "", wantID: "", wantOk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := articleIDFromItemKey(tt.itemKey)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.wantID, id)
		})
	}
}

func TestExtractEpisodeArticleIDs(t *testing.T) {
	artID1 := uuid.New().String()
	artID2 := uuid.New().String()

	episodes := []domain.TrailEpisode{
		{
			Footprints: []domain.TrailFootprint{
				{ItemKey: "article:" + artID1},
			},
		},
		{
			Footprints: []domain.TrailFootprint{},
		},
		{
			Footprints: []domain.TrailFootprint{
				{ItemKey: "feed:123"},
			},
		},
		{
			Footprints: []domain.TrailFootprint{
				{ItemKey: "article:" + artID2},
			},
		},
	}

	byEpisode, ids := extractEpisodeArticleIDs(episodes)
	assert.Equal(t, map[int]string{0: artID1, 3: artID2}, byEpisode)
	assert.Equal(t, []string{artID1, artID2}, ids)
}

func TestApplyThumbnails(t *testing.T) {
	episodes := []domain.TrailEpisode{
		{EpisodeKey: "ep-1"},
		{EpisodeKey: "ep-2"},
	}
	byEpisode := map[int]string{
		0: "art-1",
		1: "art-2",
	}
	urls := map[string]string{
		"art-1": "https://img.example.com/1.png",
	}

	result := applyThumbnails(episodes, byEpisode, urls)
	assert.Equal(t, "https://img.example.com/1.png", result[0].ThumbnailURL)
	assert.Empty(t, result[1].ThumbnailURL)
}

func TestEnrich(t *testing.T) {
	artID := uuid.New().String()
	newEpisodes := func() []domain.TrailEpisode {
		return []domain.TrailEpisode{
			{
				Footprints: []domain.TrailFootprint{
					{ItemKey: "article:" + artID},
				},
			},
		}
	}

	t.Run("empty episodes returns original", func(t *testing.T) {
		res := Enrich(context.Background(), &mockThumbnailPort{}, nil)
		assert.Nil(t, res)
	})

	t.Run("successful thumbnail enrichment", func(t *testing.T) {
		port := &mockThumbnailPort{
			urls: map[string]string{artID: "https://img.example.com/art.png"},
		}
		res := Enrich(context.Background(), port, newEpisodes())
		require.Len(t, res, 1)
		assert.Equal(t, "https://img.example.com/art.png", res[0].ThumbnailURL)
	})

	t.Run("port error degrades gracefully", func(t *testing.T) {
		port := &mockThumbnailPort{
			err: errors.New("network error"),
		}
		res := Enrich(context.Background(), port, newEpisodes())
		require.Len(t, res, 1)
		assert.Empty(t, res[0].ThumbnailURL)
	})
}
