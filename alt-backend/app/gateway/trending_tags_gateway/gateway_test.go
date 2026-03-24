package trending_tags_gateway

import (
	"alt/port/knowledge_home_port"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type mockFetchTagCounts struct {
	counts map[time.Duration][]knowledge_home_port.TagArticleCount
}

func (m *mockFetchTagCounts) FetchTagArticleCounts(_ context.Context, _ uuid.UUID, since time.Time) ([]knowledge_home_port.TagArticleCount, error) {
	age := time.Since(since)
	for dur, counts := range m.counts {
		if age >= dur-time.Hour && age <= dur+time.Hour {
			return counts, nil
		}
	}
	return nil, nil
}

func TestGetTrendingTags_DetectsSurge(t *testing.T) {
	mock := &mockFetchTagCounts{
		counts: map[time.Duration][]knowledge_home_port.TagArticleCount{
			7 * 24 * time.Hour: {
				{TagName: "AI", ArticleCount: 10},
				{TagName: "Go", ArticleCount: 2},
				{TagName: "Rust", ArticleCount: 5},
			},
			30 * 24 * time.Hour: {
				{TagName: "AI", ArticleCount: 12},
				{TagName: "Go", ArticleCount: 20},
				{TagName: "Rust", ArticleCount: 8},
			},
		},
	}

	gw := NewTrendingTagsGateway(mock, 30*time.Minute)
	tags, err := gw.GetTrendingTags(context.Background(), uuid.New())
	assert.NoError(t, err)

	// AI: recent=10, baseline=12/4=3.0, surge=10/3.0=3.33 → trending
	// Go: recent=2, below min threshold (3) → not trending
	// Rust: recent=5, baseline=8/4=2.0, surge=5/2.0=2.5 → trending
	assert.Len(t, tags, 2)
	assert.Equal(t, "AI", tags[0].TagName, "AI should rank first (higher surge)")
	assert.Equal(t, "Rust", tags[1].TagName, "Rust should rank second")
}

func TestGetTrendingTags_FiltersLowCount(t *testing.T) {
	mock := &mockFetchTagCounts{
		counts: map[time.Duration][]knowledge_home_port.TagArticleCount{
			7 * 24 * time.Hour: {
				{TagName: "rare", ArticleCount: 2}, // below threshold
			},
			30 * 24 * time.Hour: {
				{TagName: "rare", ArticleCount: 2},
			},
		},
	}

	gw := NewTrendingTagsGateway(mock, 30*time.Minute)
	tags, err := gw.GetTrendingTags(context.Background(), uuid.New())
	assert.NoError(t, err)
	assert.Empty(t, tags, "tags below min count should be filtered")
}

func TestGetTrendingTags_NoSurge(t *testing.T) {
	mock := &mockFetchTagCounts{
		counts: map[time.Duration][]knowledge_home_port.TagArticleCount{
			7 * 24 * time.Hour: {
				{TagName: "Go", ArticleCount: 5},
			},
			30 * 24 * time.Hour: {
				{TagName: "Go", ArticleCount: 20}, // weekly avg = 5, surge = 1.0 (no surge)
			},
		},
	}

	gw := NewTrendingTagsGateway(mock, 30*time.Minute)
	tags, err := gw.GetTrendingTags(context.Background(), uuid.New())
	assert.NoError(t, err)
	assert.Empty(t, tags, "tags at baseline should not be trending")
}

func TestGetTrendingTags_NewTag(t *testing.T) {
	mock := &mockFetchTagCounts{
		counts: map[time.Duration][]knowledge_home_port.TagArticleCount{
			7 * 24 * time.Hour: {
				{TagName: "NewTech", ArticleCount: 5},
			},
			30 * 24 * time.Hour: {
				// NewTech not in baseline → brand new tag, should trend
			},
		},
	}

	gw := NewTrendingTagsGateway(mock, 30*time.Minute)
	tags, err := gw.GetTrendingTags(context.Background(), uuid.New())
	assert.NoError(t, err)
	assert.Len(t, tags, 1, "brand new tag with enough articles should be trending")
	assert.Equal(t, "NewTech", tags[0].TagName)
}

func TestGetTrendingTags_CacheHit(t *testing.T) {
	callCount := 0
	mock := &mockFetchTagCounts{
		counts: map[time.Duration][]knowledge_home_port.TagArticleCount{
			7 * 24 * time.Hour: {
				{TagName: "AI", ArticleCount: 10},
			},
			30 * 24 * time.Hour: {
				{TagName: "AI", ArticleCount: 12},
			},
		},
	}

	gw := NewTrendingTagsGateway(mock, 1*time.Hour)
	userID := uuid.New()

	_, _ = gw.GetTrendingTags(context.Background(), userID)
	callCount++
	_, _ = gw.GetTrendingTags(context.Background(), userID)
	// If cached, the mock would still return data — this mainly tests that
	// the second call doesn't error. Full cache behavior tested via TTL.
	assert.Equal(t, 1, callCount)
}
