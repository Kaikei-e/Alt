package altdb

import (
	"context"
	"io"
	"log/slog"
	"testing"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// stubDataHubClient records the ListRecentArticles request and returns a
// canned response. Only the RPC under test is implemented; embedding the
// generated client interface makes every other RPC on it panic if a future
// change starts calling one from this adapter, rather than silently
// returning zero values.
type stubDataHubClient struct {
	datahubv1connect.DataHubServiceClient

	gotRequest *datahubv1.ListRecentArticlesRequest
	response   *datahubv1.ListRecentArticlesResponse
}

func (s *stubDataHubClient) ListRecentArticles(
	_ context.Context,
	req *connect.Request[datahubv1.ListRecentArticlesRequest],
) (*connect.Response[datahubv1.ListRecentArticlesResponse], error) {
	s.gotRequest = req.Msg
	return connect.NewResponse(s.response), nil
}

// The unlimited case is the one the morning-letter usecase actually runs.
// `limit` is `optional int32` precisely so that 0 ("time window only") stays
// distinguishable from unset ("server default of 100"); a nil pointer here
// would truncate the morning letter's article pool without any error.
func TestDataHubArticleClient_GetRecentArticles_SendsBothOptionalFields(t *testing.T) {
	tests := []struct {
		name            string
		withinHours     int
		limit           int
		wantWithinHours int32
		wantLimit       int32
	}{
		{
			name:            "unlimited count, 24 hour window",
			withinHours:     24,
			limit:           0,
			wantWithinHours: 24,
			wantLimit:       0,
		},
		{
			name:            "explicit count cap",
			withinHours:     6,
			limit:           25,
			wantWithinHours: 6,
			wantLimit:       25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubDataHubClient{response: &datahubv1.ListRecentArticlesResponse{}}
			client := NewDataHubArticleClient(stub, testLogger())

			_, err := client.GetRecentArticles(context.Background(), tt.withinHours, tt.limit)
			require.NoError(t, err)

			require.NotNil(t, stub.gotRequest)
			require.NotNil(t, stub.gotRequest.WithinHours,
				"within_hours must be sent explicitly, never left to the server default")
			require.NotNil(t, stub.gotRequest.Limit,
				"limit must be sent explicitly; an unset limit means 100, not unlimited")
			assert.Equal(t, tt.wantWithinHours, *stub.gotRequest.WithinHours)
			assert.Equal(t, tt.wantLimit, *stub.gotRequest.Limit)
		})
	}
}

// published_at drives temporal boost scoring downstream. An unparseable value
// must stay the zero time so scoring can deprioritize it — substituting
// time.Now() would score a broken row as "just published".
func TestDataHubArticleClient_GetRecentArticles_MapsItems(t *testing.T) {
	stub := &stubDataHubClient{
		response: &datahubv1.ListRecentArticlesResponse{
			Articles: []*datahubv1.RecentArticleItem{
				{
					Id:          "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01",
					Title:       "An LLM primer",
					Url:         "https://example.com/llm-primer",
					PublishedAt: "2026-04-14T00:30:00Z",
					FeedId:      "11111111-2222-3333-4444-555555555555",
					Tags:        []string{"ai"},
				},
				{
					Id:          "not-a-uuid",
					Title:       "Dropped",
					PublishedAt: "2026-04-14T00:30:00Z",
				},
				{
					Id:          "7f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f02",
					Title:       "Undated",
					PublishedAt: "definitely not RFC3339",
					FeedId:      "also not a uuid",
				},
			},
			Since: "2026-04-14T00:00:00Z",
			Until: "2026-04-15T00:00:00Z",
			Count: 3,
		},
	}
	client := NewDataHubArticleClient(stub, testLogger())

	articles, err := client.GetRecentArticles(context.Background(), 24, 0)
	require.NoError(t, err)

	require.Len(t, articles, 2, "the row with an unparseable id is skipped, not zero-valued")
	assert.Equal(t, "An LLM primer", articles[0].Title)
	assert.Equal(t, "https://example.com/llm-primer", articles[0].URL)
	assert.Equal(t, []string{"ai"}, articles[0].Tags)
	assert.False(t, articles[0].PublishedAt.IsZero())

	assert.True(t, articles[1].PublishedAt.IsZero(),
		"an unparseable published_at must stay zero, never be replaced with wall-clock time")
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", articles[1].FeedID.String(),
		"an unparseable feed_id falls back to the nil UUID")
}
