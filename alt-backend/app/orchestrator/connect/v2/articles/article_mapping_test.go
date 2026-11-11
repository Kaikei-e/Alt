package articles

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	articlesv2 "alt/gen/proto/alt/articles/v2"
	"alt/shared/usecase/fetch_tag_cloud_usecase"
	"alt/utils/logger"
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
			ClusterID:    2,
		},
	}

	proto := convertTagCloudItemsToProto(items)
	require.Len(t, proto, 1)
	assert.Equal(t, "golang", proto[0].TagName)
	assert.Equal(t, int32(42), proto[0].ArticleCount)
	assert.Equal(t, float32(1.5), proto[0].PositionX)
	assert.Equal(t, float32(2.5), proto[0].PositionY)
	assert.Equal(t, float32(3.5), proto[0].PositionZ)
	assert.Equal(t, int32(2), proto[0].ClusterId)
}

func TestConvertTagCloudEdgesToProto(t *testing.T) {
	edges := []*domain.TagCloudEdge{
		{
			SourceIndex: 0,
			TargetIndex: 2,
			Weight:      0.75,
		},
	}

	proto := convertTagCloudEdgesToProto(edges)
	require.Len(t, proto, 1)
	assert.Equal(t, int32(0), proto[0].SourceIndex)
	assert.Equal(t, int32(2), proto[0].TargetIndex)
	assert.InDelta(t, float32(0.75), proto[0].Weight, 1e-6)
}

type stubTagCloudPort struct {
	items         []*domain.TagCloudItem
	cooccurrences []*domain.TagCooccurrence
}

func (s *stubTagCloudPort) FetchTagCloud(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
	return s.items, nil
}

func (s *stubTagCloudPort) FetchTagCooccurrences(_ context.Context, _ []string) ([]*domain.TagCooccurrence, error) {
	return s.cooccurrences, nil
}

func TestFetchTagCloudHandler_ReturnsClustersAndEdges(t *testing.T) {
	logger.InitLogger()

	cliques := []struct {
		tags   []string
		base   int
		shared int
	}{
		{tags: []string{"rust", "tokio", "async", "wasm"}, base: 100, shared: 60},
		{tags: []string{"python", "pytorch", "llm", "transformer"}, base: 80, shared: 50},
		{tags: []string{"postgres", "sql", "index", "mvcc"}, base: 60, shared: 40},
	}

	var items []*domain.TagCloudItem
	var coocs []*domain.TagCooccurrence
	for _, c := range cliques {
		for i, name := range c.tags {
			items = append(items, &domain.TagCloudItem{
				TagName:      name,
				ArticleCount: c.base - i*5,
			})
		}
		for i := 0; i < len(c.tags); i++ {
			for j := i + 1; j < len(c.tags); j++ {
				coocs = append(coocs, &domain.TagCooccurrence{
					TagNameA:    c.tags[i],
					TagNameB:    c.tags[j],
					SharedCount: c.shared,
				})
			}
		}
	}

	port := &stubTagCloudPort{
		items:         items,
		cooccurrences: coocs,
	}

	handler := NewHandler(ArticleHandlerDeps{
		FetchTagCloud: fetch_tag_cloud_usecase.NewFetchTagCloudUsecase(port, 30*time.Minute),
	}, &config.Config{}, slog.Default())

	resp, err := handler.FetchTagCloud(createAuthContext(), connect.NewRequest(&articlesv2.FetchTagCloudRequest{Limit: 300}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Tags, 12)

	clusterByTag := make(map[string]int32, len(resp.Msg.Tags))
	for _, tag := range resp.Msg.Tags {
		assert.GreaterOrEqual(t, tag.ClusterId, int32(1))
		assert.LessOrEqual(t, tag.ClusterId, int32(8))
		clusterByTag[tag.TagName] = tag.ClusterId
	}

	distinctClusters := make(map[int32]struct{}, len(cliques))
	for _, c := range cliques {
		cid := clusterByTag[c.tags[0]]
		require.NotZero(t, cid)
		distinctClusters[cid] = struct{}{}
		for _, name := range c.tags[1:] {
			assert.Equal(t, cid, clusterByTag[name], "tags %q and %q in same clique must share ClusterId", c.tags[0], name)
		}
	}
	assert.Len(t, distinctClusters, 3)

	require.Len(t, resp.Msg.Edges, 18)
	assert.InDelta(t, float32(1.0), resp.Msg.Edges[0].Weight, 1e-6)
	for _, e := range resp.Msg.Edges {
		assert.GreaterOrEqual(t, e.SourceIndex, int32(0))
		assert.Less(t, e.SourceIndex, e.TargetIndex)
		assert.Less(t, int(e.TargetIndex), len(resp.Msg.Tags))
		assert.Greater(t, e.Weight, float32(0))
		assert.LessOrEqual(t, e.Weight, float32(1.0))
	}
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
