package metrics

import (
	"bytes"
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

func TestCoverageSampler_ChunkEmbeddingsQueryUsesCurrentVersionAndNullFilter(t *testing.T) {
	assert.Contains(t, chunkEmbeddingsQuery, "JOIN rag_documents d ON d.current_version_id = c.version_id")
	assert.Contains(t, chunkEmbeddingsQuery, "COUNT(*) - COUNT(*) FILTER (WHERE c.embedding IS NULL)")
	assert.NotContains(t, chunkEmbeddingsQuery, "count(embedding)")
}

// Null-owner rows are excluded by tenant-scoped retrieval and must be split by target pair for backfill alerting.
func TestCoverageSampler_SplitsDocumentsByOwnerStateAndWarnsWhenUnowned(t *testing.T) {
	documentCoverage.Reset()
	documentOwnerCoverage.Reset()
	var buf bytes.Buffer
	s := NewCoverageSampler(nil, time.Minute, "v10", "bge-m3/1024",
		slog.New(slog.NewJSONHandler(&buf, nil)))

	s.publish([]coverageRow{
		{chunkerVersion: "v10", embedderVersion: "bge-m3/1024", documents: 1000, unowned: 950},
		{chunkerVersion: "v9", embedderVersion: "embeddinggemma", documents: 120, unowned: 100},
	})

	assert.Equal(t, 50.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("owned", "true")))
	assert.Equal(t, 950.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("unowned", "true")))
	assert.Equal(t, 20.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("owned", "false")))
	assert.Equal(t, 100.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("unowned", "false")))

	logs := buf.String()
	assert.Contains(t, logs, `"msg":"rag_document_coverage_sampled"`)
	assert.Contains(t, logs, `"documents_unowned":1050`)
	assert.Contains(t, logs, `"documents_unowned_on_target":950`)
	assert.Contains(t, logs, `"level":"WARN"`)
	assert.Contains(t, logs, `"msg":"rag_documents_missing_owner"`)
	assert.Contains(t, logs, `"documents_owned_on_target":50`)
}

// Off-target unowned rows are orphans whose source article is gone; warning on them after backfill would never clear.
func TestCoverageSampler_UnownedOnlyOffTargetDoesNotWarn(t *testing.T) {
	documentCoverage.Reset()
	documentOwnerCoverage.Reset()
	var buf bytes.Buffer
	s := NewCoverageSampler(nil, time.Minute, "v10", "bge-m3/1024",
		slog.New(slog.NewJSONHandler(&buf, nil)))

	s.publish([]coverageRow{
		{chunkerVersion: "v10", embedderVersion: "bge-m3/1024", documents: 1000, unowned: 0},
		{chunkerVersion: "v9", embedderVersion: "embeddinggemma", documents: 120, unowned: 100},
	})

	assert.Equal(t, 1000.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("owned", "true")))
	assert.Equal(t, 0.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("unowned", "true")))
	assert.Equal(t, 20.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("owned", "false")))
	assert.Equal(t, 100.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("unowned", "false")))

	logs := buf.String()
	assert.Contains(t, logs, `"documents_unowned":100`)
	assert.Contains(t, logs, `"documents_unowned_on_target":0`)
	assert.NotContains(t, logs, "rag_documents_missing_owner")
}

// All four (state, target) series must be written even when a bucket is zero so gauges do not hold stale values.
func TestCoverageSampler_OmitsMissingOwnerWarnWhenAllOwned(t *testing.T) {
	documentCoverage.Reset()
	documentOwnerCoverage.Reset()
	var buf bytes.Buffer
	s := NewCoverageSampler(nil, time.Minute, "v10", "bge-m3/1024",
		slog.New(slog.NewJSONHandler(&buf, nil)))

	s.publish([]coverageRow{
		{chunkerVersion: "v10", embedderVersion: "bge-m3/1024", documents: 84367, unowned: 0},
	})

	assert.Equal(t, 84367.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("owned", "true")))
	assert.Equal(t, 0.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("unowned", "true")))
	assert.Equal(t, 0.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("owned", "false")))
	assert.Equal(t, 0.0, testutil.ToFloat64(documentOwnerCoverage.WithLabelValues("unowned", "false")))
	assert.NotContains(t, buf.String(), "rag_documents_missing_owner")
}

// Counting NULL user_id inside the version-pair scan avoids a second pass over rag_documents.
func TestCoverageSampler_DocumentCoverageQueryCountsNullOwnerWithFilter(t *testing.T) {
	assert.Contains(t, documentCoverageQuery, "COUNT(*) FILTER (WHERE d.user_id IS NULL)")
}
