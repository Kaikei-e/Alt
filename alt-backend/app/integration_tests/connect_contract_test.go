//go:build integration

package integration_tests

import (
	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"
	"context"
	"net/http"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFeedServiceContract_GetUnreadFeeds verifies that the Connect-RPC
// FeedService.GetUnreadFeeds endpoint returns responses conforming to the proto schema.
func TestFeedServiceContract_GetUnreadFeeds(t *testing.T) {
	addr := os.Getenv("ALT_BACKEND_CONNECT_URL")
	if addr == "" {
		addr = "http://localhost:9101"
	}

	client := feedsv2connect.NewFeedServiceClient(http.DefaultClient, addr)

	ctx := context.Background()
	resp, err := client.GetUnreadFeeds(ctx, connect.NewRequest(&feedsv2.GetUnreadFeedsRequest{
		Limit: 10,
	}))

	require.NoError(t, err, "GetUnreadFeeds should not error")
	require.NotNil(t, resp, "Response should not be nil")
	require.NotNil(t, resp.Msg, "Response message should not be nil")

	// Verify response structure conforms to proto
	msg := resp.Msg
	assert.NotNil(t, msg.Data, "Data field should be present")
	assert.IsType(t, false, msg.HasMore, "HasMore should be a bool")

	// If data is returned, verify each FeedItem has required fields
	for _, item := range msg.Data {
		assert.NotEmpty(t, item.Id, "FeedItem.Id should not be empty")
		assert.NotEmpty(t, item.Title, "FeedItem.Title should not be empty")
		assert.NotEmpty(t, item.Link, "FeedItem.Link should not be empty")
	}
}

// TestFeedServiceContract_GetFeedStats verifies feed stats endpoint.
func TestFeedServiceContract_GetFeedStats(t *testing.T) {
	addr := os.Getenv("ALT_BACKEND_CONNECT_URL")
	if addr == "" {
		addr = "http://localhost:9101"
	}

	client := feedsv2connect.NewFeedServiceClient(http.DefaultClient, addr)

	ctx := context.Background()
	resp, err := client.GetFeedStats(ctx, connect.NewRequest(&feedsv2.GetFeedStatsRequest{}))

	require.NoError(t, err, "GetFeedStats should not error")
	require.NotNil(t, resp.Msg, "Response message should not be nil")

	// Verify stats are non-negative
	assert.GreaterOrEqual(t, resp.Msg.FeedAmount, int64(0), "FeedAmount should be non-negative")
	assert.GreaterOrEqual(t, resp.Msg.SummarizedFeedAmount, int64(0), "SummarizedFeedAmount should be non-negative")
}

// TestFeedServiceContract_GetAllFeeds verifies the GetAllFeeds endpoint.
func TestFeedServiceContract_GetAllFeeds(t *testing.T) {
	addr := os.Getenv("ALT_BACKEND_CONNECT_URL")
	if addr == "" {
		addr = "http://localhost:9101"
	}

	client := feedsv2connect.NewFeedServiceClient(http.DefaultClient, addr)

	ctx := context.Background()
	resp, err := client.GetAllFeeds(ctx, connect.NewRequest(&feedsv2.GetAllFeedsRequest{
		Limit: 5,
	}))

	require.NoError(t, err, "GetAllFeeds should not error")
	require.NotNil(t, resp.Msg, "Response message should not be nil")
	assert.NotNil(t, resp.Msg.Data, "Data field should be present")
}
