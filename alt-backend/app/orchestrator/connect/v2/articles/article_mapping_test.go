package articles

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

func TestClampPageLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    int32
		def      int
		max      int
		expected int
	}{
		{"negative limit returns default", -5, 20, 99, 20},
		{"zero limit returns default", 0, 20, 99, 20},
		{"within range returns limit", 15, 20, 99, 15},
		{"exactly max returns max", 99, 20, 99, 99},
		{"exceeding max clamped to max", 150, 20, 99, 99},
		{"tag cloud custom bounds", 600, 300, 500, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampPageLimit(tt.limit, tt.def, tt.max)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDeriveRFC3339NanoCursor(t *testing.T) {
	ts := time.Date(2026, time.March, 2, 10, 0, 0, 123456789, time.UTC)
	expected := ts.Format(time.RFC3339Nano)

	tests := []struct {
		name        string
		publishedAt time.Time
		hasMore     bool
		expected    *string
	}{
		{"hasMore false returns nil", ts, false, nil},
		{"hasMore true returns formatted string", ts, true, &expected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveRFC3339NanoCursor(tt.publishedAt, tt.hasMore)
			if tt.expected == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, *tt.expected, *got)
			}
		})
	}
}

func TestDedupeValidPrefetchTargets(t *testing.T) {
	disallowedErr := errors.New("private IP")
	validateFunc := func(u *url.URL) error {
		if u.Host == "127.0.0.1" || u.Host == "localhost" {
			return disallowedErr
		}
		return nil
	}

	tests := []struct {
		name            string
		rawURLs         []string
		maxURLs         int
		validate        func(*url.URL) error
		wantValidCount  int
		wantRejCount    int
		wantSkippedSame int
	}{
		{
			name:            "empty urls",
			rawURLs:         []string{},
			maxURLs:         5,
			validate:        validateFunc,
			wantValidCount:  0,
			wantRejCount:    0,
			wantSkippedSame: 0,
		},
		{
			name: "distinct valid hosts",
			rawURLs: []string{
				"https://example.com/1",
				"https://golang.org/doc",
				"https://news.ycombinator.com/item",
			},
			maxURLs:         5,
			validate:        validateFunc,
			wantValidCount:  3,
			wantRejCount:    0,
			wantSkippedSame: 0,
		},
		{
			name: "duplicate hosts within batch deduplicated",
			rawURLs: []string{
				"https://example.com/1",
				"https://example.com/2",
				"https://example.com/3",
				"https://other.com/a",
			},
			maxURLs:         5,
			validate:        validateFunc,
			wantValidCount:  2,
			wantRejCount:    0,
			wantSkippedSame: 2,
		},
		{
			name: "disallowed and unparseable URLs rejected",
			rawURLs: []string{
				"://bad-url",
				"https://localhost/admin",
				"https://example.com/ok",
				"https://127.0.0.1/status",
			},
			maxURLs:         5,
			validate:        validateFunc,
			wantValidCount:  1,
			wantRejCount:    3,
			wantSkippedSame: 0,
		},
		{
			name: "caps raw urls at maxURLs",
			rawURLs: []string{
				"https://a.com/1",
				"https://b.com/2",
				"https://c.com/3",
				"https://d.com/4",
			},
			maxURLs:         2,
			validate:        validateFunc,
			wantValidCount:  2,
			wantRejCount:    0,
			wantSkippedSame: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, rejections, skipped := dedupeValidPrefetchTargets(tt.rawURLs, tt.maxURLs, tt.validate)
			assert.Len(t, valid, tt.wantValidCount)
			assert.Len(t, rejections, tt.wantRejCount)
			assert.Equal(t, tt.wantSkippedSame, skipped)
		})
	}
}

func TestConvertTagTrailArticlesToProto(t *testing.T) {
	now := time.Now()
	articles := []*domain.TagTrailArticle{
		{
			ID:          "art-1",
			Title:       "Title 1",
			Link:        "https://example.com/1",
			PublishedAt: now,
			FeedTitle:   "Feed 1",
		},
	}

	proto := convertTagTrailArticlesToProto(articles)
	require.Len(t, proto, 1)
	assert.Equal(t, "art-1", proto[0].Id)
	assert.Equal(t, "Title 1", proto[0].Title)
	assert.Equal(t, "https://example.com/1", proto[0].Link)
	assert.Equal(t, "Feed 1", proto[0].FeedTitle)
	assert.Equal(t, now.Format(time.RFC3339), proto[0].PublishedAt)
}

func TestConvertTagCloudItemsToProto(t *testing.T) {
	items := []*domain.TagCloudItem{
		{
			TagName:      "golang",
			ArticleCount: 42,
			PositionX:    1.5,
			PositionY:    2.5,
			PositionZ:    3.5,
		},
	}

	proto := convertTagCloudItemsToProto(items)
	require.Len(t, proto, 1)
	assert.Equal(t, "golang", proto[0].TagName)
	assert.Equal(t, int32(42), proto[0].ArticleCount)
	assert.Equal(t, float32(1.5), proto[0].PositionX)
	assert.Equal(t, float32(2.5), proto[0].PositionY)
	assert.Equal(t, float32(3.5), proto[0].PositionZ)
}

func TestConvertInoreaderSummariesToProto(t *testing.T) {
	now := time.Now()
	author := "Author X"
	summaries := []*domain.InoreaderSummary{
		{
			Title:       "Summary 1",
			Content:     "Content 1",
			Author:      &author,
			PublishedAt: now,
			FetchedAt:   now,
			InoreaderID: "ino-1",
		},
		{
			Title:       "Summary 2",
			Content:     "Content 2",
			Author:      nil,
			PublishedAt: now,
			FetchedAt:   now,
			InoreaderID: "ino-2",
		},
	}

	proto := convertInoreaderSummariesToProto(summaries)
	require.Len(t, proto, 2)
	assert.Equal(t, "Summary 1", proto[0].Title)
	assert.Equal(t, "Author X", proto[0].Author)
	assert.Equal(t, "ino-1", proto[0].SourceId)
	assert.Equal(t, "", proto[1].Author)
}
