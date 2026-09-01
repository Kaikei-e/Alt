package metrics

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSampler(targetChunker, targetEmbedder string) *CoverageSampler {
	return NewCoverageSampler(nil, time.Minute, targetChunker, targetEmbedder,
		slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

// The target label is the whole point: without it the census is a list of
// version pairs with no statement about which one is current, and 27k
// documents on a superseded embedder look exactly like 27k healthy ones.
func TestCoverageSampler_MarksTargetPair(t *testing.T) {
	documentCoverage.Reset()
	s := newTestSampler("v10", "bge-m3/1024")

	s.publish([]coverageRow{
		{chunkerVersion: "v10", embedderVersion: "bge-m3/1024", documents: 1105},
		{chunkerVersion: "v9", embedderVersion: "embeddinggemma", documents: 27927},
	})

	assert.Equal(t, 1105.0,
		testutil.ToFloat64(documentCoverage.WithLabelValues("v10", "bge-m3/1024", "true")))
	assert.Equal(t, 27927.0,
		testutil.ToFloat64(documentCoverage.WithLabelValues("v9", "embeddinggemma", "false")))
}

// A pair that has been fully rebuilt away must stop reporting. A gauge that
// keeps its last value turns a completed migration into a permanent alert.
func TestCoverageSampler_RetiresVanishedVersionPairs(t *testing.T) {
	documentCoverage.Reset()
	s := newTestSampler("v10", "bge-m3/1024")

	s.publish([]coverageRow{
		{chunkerVersion: "v9", embedderVersion: "v1", documents: 21849},
		{chunkerVersion: "v10", embedderVersion: "bge-m3/1024", documents: 10},
	})
	require.Equal(t, 2, testutil.CollectAndCount(documentCoverage))

	s.publish([]coverageRow{
		{chunkerVersion: "v10", embedderVersion: "bge-m3/1024", documents: 21859},
	})

	assert.Equal(t, 1, testutil.CollectAndCount(documentCoverage),
		"the drained version pair must be dropped, not frozen at its last count")
	assert.Equal(t, 21859.0,
		testutil.ToFloat64(documentCoverage.WithLabelValues("v10", "bge-m3/1024", "true")))
}

// A NULL embedding is invisible to the document census — the version stamp
// says the document was indexed either way — and invisible to vector search.
// Splitting the chunk count is the only place that state shows up.
func TestCoverageSampler_SplitsChunksByEmbeddingState(t *testing.T) {
	chunkEmbeddingCoverage.Reset()
	s := newTestSampler("v10", "bge-m3/1024")

	s.publishChunkEmbeddings(2177875, 29022)

	assert.Equal(t, 29022.0, testutil.ToFloat64(chunkEmbeddingCoverage.WithLabelValues("embedded")))
	assert.Equal(t, 2148853.0, testutil.ToFloat64(chunkEmbeddingCoverage.WithLabelValues("missing")))
}

// The scrape timestamp is what distinguishes "the corpus is healthy" from
// "the sampler died an hour ago and the numbers are stale".
func TestCoverageSampler_AdvancesScrapeTimestamp(t *testing.T) {
	documentCoverage.Reset()
	coverageScrapeTimestamp.Set(0)
	s := newTestSampler("v10", "bge-m3/1024")

	s.publish([]coverageRow{{chunkerVersion: "v10", embedderVersion: "bge-m3/1024", documents: 1}})

	assert.Greater(t, testutil.ToFloat64(coverageScrapeTimestamp), 0.0)
}
