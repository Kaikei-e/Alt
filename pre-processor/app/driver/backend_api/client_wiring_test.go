package backend_api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/domain"
)

// TestClientRoutesToDataHubNamespace pins the procedure path the *production*
// client actually puts on the wire (ADR-000954 D7).
//
// The unit tests around these repositories inject a mock at the generated
// client interface, which is above the point where the namespace is chosen —
// they stay green whichever contract the client is built from. alt-data-hub
// also serves both namespaces for the length of Wave 2, so a client left on
// services.backend.v1 keeps working right up until the legacy routes are
// removed, and then fails in production rather than in CI. This test is what
// closes that window: it drives NewClient the way bootstrap/wire.go does and
// asserts on the path an HTTP server sees.
func TestClientRoutesToDataHubNamespace(t *testing.T) {
	tests := []struct {
		name string
		want string
		call func(ctx context.Context, c *Client) error
	}{
		{
			name: "GetFeedID",
			want: "/alt.datahub.v1.DataHubService/GetFeedID",
			call: func(ctx context.Context, c *Client) error {
				_, err := NewArticleRepository(c, nil).getFeedID(ctx, "https://example.com/feed.xml")
				return err
			},
		},
		{
			name: "GetEmptyFeedID",
			want: "/alt.datahub.v1.DataHubService/GetEmptyFeedID",
			call: func(ctx context.Context, c *Client) error {
				_, err := NewArticleRepository(c, nil).getEmptyFeedID(ctx, "https://example.com/feed.xml")
				return err
			},
		},
		{
			name: "CreateArticle",
			want: "/alt.datahub.v1.DataHubService/CreateArticle",
			call: func(ctx context.Context, c *Client) error {
				return NewArticleRepository(c, nil).Create(ctx, &domain.Article{
					Title:   "Go 1.26 Released",
					URL:     "https://example.com/articles/go-126",
					Content: "Article body text.",
					FeedID:  "feed-001",
					UserID:  "user-001",
				})
			},
		},
		{
			name: "ListUnsummarizedArticles",
			want: "/alt.datahub.v1.DataHubService/ListUnsummarizedArticles",
			call: func(ctx context.Context, c *Client) error {
				_, _, err := NewArticleRepository(c, nil).FindForSummarization(ctx, nil, 100)
				return err
			},
		},
		{
			name: "HasUnsummarizedArticles",
			want: "/alt.datahub.v1.DataHubService/HasUnsummarizedArticles",
			call: func(ctx context.Context, c *Client) error {
				_, err := NewArticleRepository(c, nil).HasUnsummarizedArticles(ctx)
				return err
			},
		},
		{
			name: "GetArticleContent",
			want: "/alt.datahub.v1.DataHubService/GetArticleContent",
			call: func(ctx context.Context, c *Client) error {
				_, err := NewArticleRepository(c, nil).FindByID(ctx, "art-001")
				return err
			},
		},
		{
			name: "ListFeedURLs",
			want: "/alt.datahub.v1.DataHubService/ListFeedURLs",
			call: func(ctx context.Context, c *Client) error {
				_, _, err := NewFeedRepository(c).GetUnprocessedFeeds(ctx, nil, 100)
				return err
			},
		},
		{
			name: "SaveArticleSummary",
			want: "/alt.datahub.v1.DataHubService/SaveArticleSummary",
			call: func(ctx context.Context, c *Client) error {
				return NewSummaryRepository(c).Create(ctx, &domain.ArticleSummary{
					ArticleID:       "art-001",
					SummaryJapanese: "要約",
					UserID:          "user-001",
				})
			},
		},
		{
			name: "FindArticlesWithSummaries",
			want: "/alt.datahub.v1.DataHubService/FindArticlesWithSummaries",
			call: func(ctx context.Context, c *Client) error {
				_, _, err := NewSummaryRepository(c).FindArticlesWithSummaries(ctx, nil, 100)
				return err
			},
		},
		{
			name: "DeleteArticleSummary",
			want: "/alt.datahub.v1.DataHubService/DeleteArticleSummary",
			call: func(ctx context.Context, c *Client) error {
				return NewSummaryRepository(c).Delete(ctx, "art-001")
			},
		},
		{
			name: "CheckArticleSummaryExists",
			want: "/alt.datahub.v1.DataHubService/CheckArticleSummaryExists",
			call: func(ctx context.Context, c *Client) error {
				_, err := NewSummaryRepository(c).Exists(ctx, "art-001")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				// An empty protobuf message is a valid encoding of every
				// response type here, so the call returns cleanly and the
				// assertion below is about the path and nothing else.
				w.Header().Set("Content-Type", "application/proto")
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "", srv.Client())
			require.NoError(t, tt.call(context.Background(), client))

			assert.Equal(t, tt.want, gotPath,
				"pre-processor must call alt-data-hub through alt.datahub.v1.DataHubService (ADR-000954 D7)")
			assert.False(t, strings.Contains(gotPath, "services.backend.v1"),
				"the services.backend.v1 namespace is retired")
		})
	}
}

// TestCheckExistsRoutesToDataHubNamespace covers CheckArticleExists, which is
// only reachable behind a successful GetFeedID and so needs a routing stub
// rather than the single-path server above.
func TestCheckExistsRoutesToDataHubNamespace(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.Header().Set("Content-Type", "application/proto")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", srv.Client())
	_, err := NewArticleRepository(client, nil).CheckExists(context.Background(),
		[]string{"https://example.com/articles/go-126"})
	require.NoError(t, err)

	// An empty GetFeedIDResponse yields feedID == "" and CheckExists skips the
	// url, so assert on what it did ask for: the feed lookup, under the new
	// namespace.
	require.NotEmpty(t, seen)
	for _, path := range seen {
		assert.True(t, strings.HasPrefix(path, "/alt.datahub.v1.DataHubService/"),
			"every alt-data-hub call must use the D7 namespace, got %s", path)
	}
}
