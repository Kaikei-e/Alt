package opml_gateway

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The previous tests here asserted that a nil *AltDBRepository produced
// "database connection not available". With the bulk registration served by
// alt-data-hub there is no repository to be nil, and the behaviour worth
// pinning is what this gateway still owns: the SSRF check, the tracking
// parameter strip, and the batch-level dedupe those two make possible.

type bulkStoreStub struct {
	gotURLs []string
	result  *domain.OPMLImportResult
}

func (s *bulkStoreStub) RegisterFeedLinkBulk(_ context.Context, urls []string) (*domain.OPMLImportResult, error) {
	s.gotURLs = urls
	if s.result != nil {
		return s.result, nil
	}
	return &domain.OPMLImportResult{Total: len(urls), Imported: len(urls)}, nil
}

func TestImportGateway_RejectsEmptyURLBeforeTheDataPlane(t *testing.T) {
	logger.InitLogger()
	store := &bulkStoreStub{}
	gateway := NewImportGateway(store)

	result, err := gateway.RegisterFeedLinkBulk(context.Background(), []string{""})

	require.NoError(t, err)
	assert.Equal(t, 1, result.Failed)
	assert.Empty(t, store.gotURLs, "an empty outline must never reach the data plane")
}

// Two URLs differing only in tracking parameters are one subscription. The
// strip happens here — it is a pure function over the string (ADR-000954 D4) —
// and the dedupe is only possible after it, which is why both stay on this
// side rather than being pushed into the batch procedure.
func TestImportGateway_DedupesAfterStrippingTrackingParams(t *testing.T) {
	logger.InitLogger()
	store := &bulkStoreStub{}
	gateway := NewImportGateway(store)

	result, err := gateway.RegisterFeedLinkBulk(context.Background(), []string{
		"https://example.com/feed?utm_source=rss",
		"https://example.com/feed?utm_source=chatgpt",
	})

	require.NoError(t, err)
	assert.Equal(t, 2, result.Total, "Total counts the outlines the user submitted")
	require.Len(t, store.gotURLs, 1, "the two UTM variants collapse to one subscription")
	assert.Equal(t, 1, result.Skipped, "the collapsed duplicate is reported as skipped, not imported")
}

// The provider's counts are merged into the ones this gateway produced rather
// than replacing them: outlines rejected here never reach the data plane, so
// its response knows nothing about them.
func TestImportGateway_MergesProviderCountsWithLocalRejections(t *testing.T) {
	logger.InitLogger()
	store := &bulkStoreStub{result: &domain.OPMLImportResult{
		Imported:   1,
		Skipped:    1,
		Failed:     1,
		FailedURLs: []string{"https://example.com/c.xml"},
	}}
	gateway := NewImportGateway(store)

	result, err := gateway.RegisterFeedLinkBulk(context.Background(), []string{
		"",
		"https://example.com/a.xml",
		"https://example.com/b.xml",
		"https://example.com/c.xml",
	})

	require.NoError(t, err)
	assert.Equal(t, 4, result.Total)
	assert.Equal(t, 1, result.Imported)
	assert.Equal(t, 1, result.Skipped)
	assert.Equal(t, 2, result.Failed, "one rejected locally plus one rejected by the provider")
	assert.Contains(t, result.FailedURLs, "https://example.com/c.xml")
}

func TestNewImportGateway_RefusesNilStore(t *testing.T) {
	assert.Panics(t, func() { NewImportGateway(nil) })
}
