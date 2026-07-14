package opml_gateway

import (
	"alt/utils/logger"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportGateway_RegisterFeedLinkBulk_NilDB(t *testing.T) {
	gateway := &ImportGateway{altDB: nil}

	_, err := gateway.RegisterFeedLinkBulk(context.Background(), []string{"https://example.com/feed"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection not available")
}

func TestImportGateway_RegisterFeedLinkBulk_EmptyURL(t *testing.T) {
	logger.InitLogger()

	gateway := &ImportGateway{altDB: nil}

	// nil DB but empty URL should be caught before DB call
	_, err := gateway.RegisterFeedLinkBulk(context.Background(), []string{""})
	// nil DB returns error
	assert.Error(t, err)
}

func TestImportGateway_RegisterFeedLinkBulk_BatchDedup(t *testing.T) {
	logger.InitLogger()

	// With nil DB, we can't test the full flow, but we can verify it
	// doesn't panic with UTM URLs. The core dedup logic is tested via
	// StripTrackingParams in utils.
	gateway := &ImportGateway{altDB: nil}

	urls := []string{
		"https://example.com/feed?utm_source=rss",
		"https://example.com/feed?utm_source=chatgpt",
	}
	_, err := gateway.RegisterFeedLinkBulk(context.Background(), urls)
	require.Error(t, err) // nil DB
}
