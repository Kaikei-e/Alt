package repository

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backend_api "pre-processor/driver/backend_api"
	datahubv1 "pre-processor/gen/proto/services/datahub/v1"
	"pre-processor/gen/proto/services/datahub/v1/datahubv1connect"
)

type mockFeedDataHubClient struct {
	datahubv1connect.UnimplementedDataHubServiceHandler
	listFeedURLsFunc func(ctx context.Context, req *connect.Request[datahubv1.ListFeedURLsRequest]) (*connect.Response[datahubv1.ListFeedURLsResponse], error)
}

func (m *mockFeedDataHubClient) ListFeedURLs(ctx context.Context, req *connect.Request[datahubv1.ListFeedURLsRequest]) (*connect.Response[datahubv1.ListFeedURLsResponse], error) {
	return m.listFeedURLsFunc(ctx, req)
}

func TestFeedRepository_GetUnprocessedFeeds_PreservesEmptyURL(t *testing.T) {
	mock := &mockFeedDataHubClient{
		listFeedURLsFunc: func(_ context.Context, _ *connect.Request[datahubv1.ListFeedURLsRequest]) (*connect.Response[datahubv1.ListFeedURLsResponse], error) {
			return connect.NewResponse(&datahubv1.ListFeedURLsResponse{
				Feeds: []*datahubv1.FeedURL{
					{Url: "https://example.com/a.xml"},
					{Url: ""},
					{Url: "https://example.com/b.xml"},
				},
			}), nil
		},
	}

	repo := NewFeedRepository(backend_api.NewClientWithService(mock))
	urls, cursor, err := repo.GetUnprocessedFeeds(t.Context(), nil, 100)
	require.NoError(t, err)
	assert.Nil(t, cursor)
	require.Len(t, urls, 3)
	assert.Equal(t, "https://example.com/a.xml", urls[0].String())
	assert.Empty(t, urls[1].String())
	assert.Equal(t, "https://example.com/b.xml", urls[2].String())
}
