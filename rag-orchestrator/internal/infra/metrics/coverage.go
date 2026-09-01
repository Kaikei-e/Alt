package metrics

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// documentCoverage is the corpus census: how many documents currently sit
	// on each (chunker_version, embedder_version) pair, and whether that pair
	// is the one this deployment writes today.
	//
	// Corpus rot is invisible from the request path. A document embedded by a
	// superseded model still returns from a query, still scores, still gets
	// cited — it is simply ranked against vectors from a different space. The
	// only way to see it is to count what the store holds and compare it to
	// what the service is configured to produce.
	documentCoverage = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: "rag",
		Name:      "document_coverage",
		Help: "Documents (current version) per (chunker_version, " +
			"embedder_version). target=\"true\" marks the pair this deployment " +
			"currently produces.",
	}, []string{"chunker_version", "embedder_version", "target"})

	// coverageScrapeTimestamp goes stale when the sampler stops, which a
	// gauge of counts cannot show: the last-known numbers would simply stay
	// on the dashboard looking healthy.
	coverageScrapeTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: "rag",
		Name:      "document_coverage_scrape_timestamp_seconds",
		Help:      "Unix timestamp of the last successful coverage sample.",
	})

	// chunkEmbeddingCoverage counts chunks that carry a vector against chunks
	// that do not. The document census above cannot see this: a document is
	// stamped with the embedder version it was indexed under whether or not
	// the embedding survived, so a corpus can report a healthy version split
	// while almost none of its chunks are reachable by vector search at all.
	chunkEmbeddingCoverage = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: "rag",
		Name:      "chunk_embedding_coverage",
		Help: "Chunks by embedding state (embedded, missing). A chunk with a " +
			"NULL embedding is retrievable lexically and invisible to vector search.",
	}, []string{"state"})

	coverageScrapeFailures = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: "rag",
		Name:      "document_coverage_scrape_failures_total",
		Help:      "Coverage sampling attempts that failed to query rag-db.",
	})
)

// CoverageSampler periodically counts indexed documents by version pair.
type CoverageSampler struct {
	pool            *pgxpool.Pool
	interval        time.Duration
	targetChunker   string
	targetEmbedder  string
	logger          *slog.Logger
	stop            chan struct{}
	stopped         chan struct{}
	previousLabels  map[[2]string]struct{}
	queryTimeoutSec int
}

// NewCoverageSampler builds the sampler. targetChunker / targetEmbedder are the
// versions this deployment writes today — they come from the same chunker and
// embedder the index path uses, so a config change moves the target on its own
// rather than waiting for someone to update a constant.
func NewCoverageSampler(
	pool *pgxpool.Pool,
	interval time.Duration,
	targetChunker, targetEmbedder string,
	logger *slog.Logger,
) *CoverageSampler {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	return &CoverageSampler{
		pool:            pool,
		interval:        interval,
		targetChunker:   targetChunker,
		targetEmbedder:  targetEmbedder,
		logger:          logger,
		stop:            make(chan struct{}),
		stopped:         make(chan struct{}),
		previousLabels:  make(map[[2]string]struct{}),
		queryTimeoutSec: 30,
	}
}

// Start samples once immediately, then on the configured interval. The first
// sample runs inline so a freshly started container publishes a real corpus
// state instead of nothing for the first interval.
func (s *CoverageSampler) Start(ctx context.Context) {
	s.logger.Info("rag_document_coverage_sampler_enabled",
		slog.Duration("interval", s.interval),
		slog.String("target_chunker_version", s.targetChunker),
		slog.String("target_embedder_version", s.targetEmbedder))

	go func() {
		defer close(s.stopped)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.sampleOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stop:
				return
			case <-ticker.C:
				s.sampleOnce(ctx)
			}
		}
	}()
}

// Stop ends the sampling loop and waits for it to finish.
func (s *CoverageSampler) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	<-s.stopped
}

type coverageRow struct {
	chunkerVersion  string
	embedderVersion string
	documents       int64
}

func (s *CoverageSampler) sampleOnce(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(s.queryTimeoutSec)*time.Second)
	defer cancel()

	rows, err := s.queryCoverage(queryCtx)
	if err != nil {
		coverageScrapeFailures.Inc()
		s.logger.Error("rag_document_coverage_sample_failed",
			slog.String("error", err.Error()))
		return
	}
	s.publish(rows)

	total, embedded, err := s.queryChunkEmbeddings(queryCtx)
	if err != nil {
		coverageScrapeFailures.Inc()
		s.logger.Error("rag_chunk_embedding_sample_failed",
			slog.String("error", err.Error()))
		return
	}
	s.publishChunkEmbeddings(total, embedded)
}

func (s *CoverageSampler) publishChunkEmbeddings(total, embedded int64) {
	chunkEmbeddingCoverage.WithLabelValues("embedded").Set(float64(embedded))
	chunkEmbeddingCoverage.WithLabelValues("missing").Set(float64(total - embedded))
	s.logger.Info("rag_chunk_embedding_sampled",
		slog.Int64("chunks_total", total),
		slog.Int64("chunks_embedded", embedded))
}

// queryChunkEmbeddings counts chunks with and without a vector. count(embedding)
// has to touch the heap, so this is paced by the sampler interval rather than
// scraped on demand.
func (s *CoverageSampler) queryChunkEmbeddings(ctx context.Context) (total, embedded int64, err error) {
	const q = `SELECT count(*), count(embedding) FROM rag_chunks`
	err = s.pool.QueryRow(ctx, q).Scan(&total, &embedded)
	return total, embedded, err
}

func (s *CoverageSampler) publish(rows []coverageRow) {
	// Retire label sets that no longer exist, or a version fully rebuilt away
	// keeps reporting its last count forever.
	current := make(map[[2]string]struct{}, len(rows))
	var total, onTarget int64
	for _, row := range rows {
		key := [2]string{row.chunkerVersion, row.embedderVersion}
		current[key] = struct{}{}
		isTarget := row.chunkerVersion == s.targetChunker && row.embedderVersion == s.targetEmbedder
		documentCoverage.
			WithLabelValues(row.chunkerVersion, row.embedderVersion, strconv.FormatBool(isTarget)).
			Set(float64(row.documents))
		total += row.documents
		if isTarget {
			onTarget += row.documents
		}
	}
	for key := range s.previousLabels {
		if _, still := current[key]; still {
			continue
		}
		isTarget := key[0] == s.targetChunker && key[1] == s.targetEmbedder
		documentCoverage.DeleteLabelValues(key[0], key[1], strconv.FormatBool(isTarget))
	}
	s.previousLabels = current

	coverageScrapeTimestamp.Set(float64(time.Now().Unix()))
	s.logger.Info("rag_document_coverage_sampled",
		slog.Int("version_pairs", len(rows)),
		slog.Int64("documents_total", total),
		slog.Int64("documents_on_target", onTarget),
		slog.String("target_chunker_version", s.targetChunker),
		slog.String("target_embedder_version", s.targetEmbedder))
}

// queryCoverage counts each document once, through its current version only:
// superseded versions are history, not corpus.
func (s *CoverageSampler) queryCoverage(ctx context.Context) ([]coverageRow, error) {
	const q = `
		SELECT v.chunker_version, v.embedder_version, COUNT(*)
		FROM rag_documents d
		JOIN rag_document_versions v ON d.current_version_id = v.id
		GROUP BY v.chunker_version, v.embedder_version
	`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []coverageRow
	for rows.Next() {
		var r coverageRow
		if err := rows.Scan(&r.chunkerVersion, &r.embedderVersion, &r.documents); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
