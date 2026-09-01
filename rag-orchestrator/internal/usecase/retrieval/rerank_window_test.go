package retrieval_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingReranker struct{}

func (failingReranker) Rerank(context.Context, string, []domain.RerankCandidate) ([]domain.RerankResult, error) {
	return nil, errors.New("rerank server returned 500")
}

func (failingReranker) ModelName() string { return "failing-reranker" }

// emptyReranker answers with no scores at all — a 200 with an empty body, which
// is indistinguishable from "every candidate scored 0" unless it is caught.
type emptyReranker struct{}

func (emptyReranker) Rerank(context.Context, string, []domain.RerankCandidate) ([]domain.RerankResult, error) {
	return nil, nil
}

func (emptyReranker) ModelName() string { return "empty-reranker" }

// captureLogger returns a logger writing JSON records into buf.
func captureLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// logRecords parses the captured JSON lines into maps.
func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for dec.More() {
		var rec map[string]any
		require.NoError(t, dec.Decode(&rec))
		out = append(out, rec)
	}
	return out
}

func findRecord(records []map[string]any, msg string) map[string]any {
	for _, rec := range records {
		if rec["msg"] == msg {
			return rec
		}
	}
	return nil
}

func hitsWithScores(n int) []domain.SearchResult {
	hits := make([]domain.SearchResult, n)
	for i := range hits {
		hits[i] = domain.SearchResult{
			Chunk:     domain.RagChunk{ID: uuid.New(), Content: fmt.Sprintf("chunk %d", i)},
			ArticleID: fmt.Sprintf("art-%d", i),
			Title:     fmt.Sprintf("Article %d", i),
			Score:     float32(n-i) / float32(n),
			ScoreKind: domain.ScoreKindRRF,
		}
	}
	return hits
}

// TestRerank_SendsTheWholeWindowToTheCrossEncoder is the point of reranking:
// the stage used to cut candidates to TopK *before* the call, so a hit ranked
// 11th by RRF could never be rescued no matter how relevant the cross-encoder
// judged it. The candidate window and the output size are different numbers.
func TestRerank_SendsTheWholeWindowToTheCrossEncoder(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:  "rerank-window",
		Query:        "why did this happen",
		HitsOriginal: hitsWithScores(25),
	}
	reranker := &capturingReranker{}

	retrieval.Rerank(context.Background(), sc, reranker, retrieval.RerankConfig{
		Enabled:       true,
		TopK:          10,
		MaxCandidates: 40,
		Timeout:       time.Second,
	}, discardLogger())

	assert.Len(t, reranker.got, 25,
		"every candidate up to MaxCandidates must reach the cross-encoder, not just TopK")
}

// TestRerank_TruncatesToTopKAfterScoring is the other half: TopK now shapes the
// output, so the stage genuinely reranks a wide pool down to a narrow one.
func TestRerank_TruncatesToTopKAfterScoring(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:  "rerank-topk",
		Query:        "why did this happen",
		HitsOriginal: hitsWithScores(25),
	}

	retrieval.Rerank(context.Background(), sc, &capturingReranker{}, retrieval.RerankConfig{
		Enabled:       true,
		TopK:          5,
		MaxCandidates: 40,
		Timeout:       time.Second,
	}, discardLogger())

	assert.Len(t, sc.HitsOriginal, 5, "the reranked pool must be cut to TopK after scoring")
}

// TestRerank_CapsCandidatesAtMaxCandidates keeps the CPU reranker's measured
// capacity in the loop: the window is bounded, just not by TopK.
func TestRerank_CapsCandidatesAtMaxCandidates(t *testing.T) {
	sc := &retrieval.StageContext{
		RetrievalID:  "rerank-cap",
		Query:        "why did this happen",
		HitsOriginal: hitsWithScores(60),
	}
	reranker := &capturingReranker{}

	retrieval.Rerank(context.Background(), sc, reranker, retrieval.RerankConfig{
		Enabled:       true,
		TopK:          10,
		MaxCandidates: 40,
		Timeout:       time.Second,
	}, discardLogger())

	require.Len(t, reranker.got, 40)
	assert.Equal(t, "chunk 0", reranker.got[0].Content,
		"the window must keep the best-scoring candidates, in retrieval order")
}

// TestRerank_MixedHits_KeepsDedupIdentity preserves the behaviour rerank_test.go
// pins: BM25 hits carry no chunk id, so identity falls back to the article id.
func TestRerank_MixedHits_KeepsDedupIdentity(t *testing.T) {
	sharedChunk := uuid.New()
	sc := &retrieval.StageContext{
		RetrievalID: "rerank-dedup",
		Query:       "why did this happen",
		HitsOriginal: []domain.SearchResult{
			{Chunk: domain.RagChunk{ID: sharedChunk, Content: "a"}, ArticleID: "art-1", Score: 0.9, ScoreKind: domain.ScoreKindRRF},
			{Chunk: domain.RagChunk{Content: "b"}, ArticleID: "art-2", Score: 0.8, ScoreKind: domain.ScoreKindBM25},
		},
		HitsExpanded: []retrieval.ContextItem{
			// Same chunk as the first original hit: one candidate, not two.
			{ChunkID: sharedChunk, ChunkText: "a", ArticleID: "art-1", Score: 0.7, ScoreKind: domain.ScoreKindRRF},
		},
	}
	reranker := &capturingReranker{}

	retrieval.Rerank(context.Background(), sc, reranker, retrieval.RerankConfig{
		Enabled:       true,
		TopK:          10,
		MaxCandidates: 40,
		Timeout:       time.Second,
	}, discardLogger())

	assert.Len(t, reranker.got, 2, "a chunk seen in both lists is a single candidate")
}

// TestRerank_Failure_LogsLoudly: falling back to retrieval scores is a quality
// degradation, and CLAUDE.md rule 8 forbids it happening quietly. The record
// has to carry enough to tell a timeout apart from a server fault without a
// second look at the reranker's own logs.
func TestRerank_Failure_LogsLoudly(t *testing.T) {
	var buf bytes.Buffer
	sc := &retrieval.StageContext{
		RetrievalID:  "rerank-loud",
		Query:        "why did this happen",
		HitsOriginal: hitsWithScores(12),
	}

	retrieval.Rerank(context.Background(), sc, failingReranker{}, retrieval.RerankConfig{
		Enabled:       true,
		TopK:          10,
		MaxCandidates: 40,
		Timeout:       2 * time.Second,
	}, captureLogger(&buf))

	rec := findRecord(logRecords(t, &buf), "rerank_fallback_original_scores")
	require.NotNil(t, rec, "a rerank fallback must be logged under a searchable event name")
	assert.Equal(t, "ERROR", rec["level"], "a silent quality degradation is the failure mode this guards")
	assert.EqualValues(t, 12, rec["candidate_count"])
	assert.Equal(t, "rerank_skipped", rec["degraded_mode"])
	assert.Contains(t, rec, "duration_ms")
	assert.EqualValues(t, 2000, rec["timeout_ms"],
		"the configured client timeout belongs in the record: a client that gives up before the server does looks exactly like a server fault")
	assert.False(t, sc.RerankApplied)
	assert.Len(t, sc.HitsOriginal, 12, "the retrieval order survives a reranker outage")
}

// TestRerank_EmptyResult_IsTreatedAsFailure: a reranker that scores nothing
// must not be allowed to empty the pipeline through the TopK truncation.
func TestRerank_EmptyResult_IsTreatedAsFailure(t *testing.T) {
	var buf bytes.Buffer
	sc := &retrieval.StageContext{
		RetrievalID:  "rerank-empty",
		Query:        "why did this happen",
		HitsOriginal: hitsWithScores(12),
	}

	retrieval.Rerank(context.Background(), sc, emptyReranker{}, retrieval.RerankConfig{
		Enabled:       true,
		TopK:          5,
		MaxCandidates: 40,
		Timeout:       time.Second,
	}, captureLogger(&buf))

	assert.False(t, sc.RerankApplied)
	assert.Len(t, sc.HitsOriginal, 12)
	assert.NotNil(t, findRecord(logRecords(t, &buf), "rerank_fallback_original_scores"))
}

// TestRerank_EnabledWithoutReranker_LogsTheWiringGap distinguishes "operator
// turned reranking off" from "DI forgot to wire the client" (ADR-000928).
func TestRerank_EnabledWithoutReranker_LogsTheWiringGap(t *testing.T) {
	var buf bytes.Buffer
	sc := &retrieval.StageContext{
		RetrievalID:  "rerank-unwired",
		Query:        "why did this happen",
		HitsOriginal: hitsWithScores(3),
	}

	retrieval.Rerank(context.Background(), sc, nil, retrieval.RerankConfig{
		Enabled:       true,
		TopK:          10,
		MaxCandidates: 40,
		Timeout:       time.Second,
	}, captureLogger(&buf))

	rec := findRecord(logRecords(t, &buf), "rerank_enabled_but_unwired")
	require.NotNil(t, rec)
	assert.Equal(t, "ERROR", rec["level"])
	assert.False(t, sc.RerankApplied)
}
