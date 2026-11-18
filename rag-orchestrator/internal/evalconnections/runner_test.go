package evalconnections

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSimilarityPort struct {
	vectors map[string]map[domain.SimilarityRepresentation][][]float32
	calls   int
}

func newFakeSimilarityPort(n int) *fakeSimilarityPort {
	vecs := make(map[string]map[domain.SimilarityRepresentation][][]float32, n)
	for i := 0; i < n; i++ {
		id := filepath.Base(string(rune('a'+(i/26))) + string(rune('a'+(i%26))))
		_ = id
	}
	return &fakeSimilarityPort{vectors: vecs}
}

func (f *fakeSimilarityPort) ComputeCandidateSimilarities(
	_ context.Context,
	seedArticleID string,
	candidateArticleIDs []string,
	rep domain.SimilarityRepresentation,
	_ uuid.UUID,
) (*domain.CandidateSimilarityResult, error) {
	f.calls++
	seedVecs, ok := f.vectors[seedArticleID]
	if !ok {
		return &domain.CandidateSimilarityResult{
			SeedIndexed:                false,
			CandidatesWithoutEmbedding: len(candidateArticleIDs),
			Results:                    []domain.CandidateSimilarityItem{},
		}, nil
	}

	var results []domain.CandidateSimilarityItem
	missing := 0
	for _, cid := range candidateArticleIDs {
		cVecs, ok := f.vectors[cid]
		if !ok {
			missing++
			continue
		}
		sim := computeVecSim(seedVecs[rep], cVecs[rep])
		results = append(results, domain.CandidateSimilarityItem{
			ArticleID:  cid,
			Similarity: sim,
		})
	}
	return &domain.CandidateSimilarityResult{
		SeedIndexed:                true,
		CandidatesWithoutEmbedding: missing,
		Results:                    results,
	}, nil
}

func computeVecSim(a, b [][]float32) float64 {
	best := -2.0
	for _, va := range a {
		for _, vb := range b {
			s, err := CosineSimilarity(va, vb)
			if err == nil && s > best {
				best = s
			}
		}
	}
	return best
}

type fakeMetadataPort struct {
	meta map[string]domain.RagDocumentMetadata
}

func (f *fakeMetadataPort) GetArticleMetadataByIDs(_ context.Context, articleIDs []string, _ uuid.UUID) (map[string]domain.RagDocumentMetadata, error) {
	out := make(map[string]domain.RagDocumentMetadata, len(articleIDs))
	for _, id := range articleIDs {
		if m, ok := f.meta[id]; ok {
			out[id] = m
		}
	}
	return out, nil
}

type fakeSummaryEncoder struct {
	calls int
}

func (f *fakeSummaryEncoder) Encode(_ context.Context, texts []string) ([][]float32, error) {
	f.calls++
	out := make([][]float32, len(texts))
	for i, text := range texts {
		angle := float64(len(text)%36) * math.Pi / 18.0
		out[i] = []float32{float32(math.Cos(angle)), float32(math.Sin(angle))}
	}
	return out, nil
}

func (f *fakeSummaryEncoder) Version() string { return "fake/2" }

func TestSampleRunner_EndToEndWithFakes(t *testing.T) {
	const count = 40
	port := newFakeSimilarityPort(count)
	metaPort := &fakeMetadataPort{meta: make(map[string]domain.RagDocumentMetadata, count)}
	readIDs := make([]string, 0, count+1)

	for i := 0; i < count; i++ {
		id := "art-" + string([]byte{'0' + byte(i/10), '0' + byte(i%10)})
		readIDs = append(readIDs, id)
		a1 := float64(i) * 0.15
		a2 := float64((i*7)%count) * 0.15
		port.vectors[id] = map[domain.SimilarityRepresentation][][]float32{
			domain.SimilarityRepChunkMean:        {{float32(math.Cos(a1)), float32(math.Sin(a1))}},
			domain.SimilarityRepFirstChunk:       {{float32(math.Cos(a2)), float32(math.Sin(a2))}},
			domain.SimilarityRepNearestChunkPair: {{float32(math.Cos(a1)), float32(math.Sin(a1))}, {float32(math.Cos(a2)), float32(math.Sin(a2))}},
		}
		metaPort.meta[id] = domain.RagDocumentMetadata{
			ArticleID: id,
			Title:     "DB Title " + id,
			URL:       "https://example.com/" + id,
		}
	}
	// Include one unindexed read article to verify filtering.
	readIDs = append(readIDs, "art-unindexed")

	uc := usecase.NewCandidateSimilarityUsecase(port)
	outDir := t.TempDir()

	t.Run("without summaries skips summary_vector", func(t *testing.T) {
		runner := NewSampleRunner(uc, metaPort, nil)
		res, err := runner.Run(context.Background(), SampleOptions{
			UserID:  uuid.NewString(),
			ReadIDs: readIDs,
			OutDir:  outDir,
			Seed:    123,
			TitleOverrides: map[string]domain.RagDocumentMetadata{
				"art-00": {ArticleID: "art-00", Title: "Override Title 00", URL: "https://override.example/00"},
			},
		})
		require.NoError(t, err)
		assert.Equal(t, TotalSamplePairs, res.PairCount)

		sheetFile, err := os.Open(res.SheetPath)
		require.NoError(t, err)
		defer func() { _ = sheetFile.Close() }()
		rows, err := ReadScoringSheetCSV(sheetFile)
		require.NoError(t, err)
		require.Len(t, rows, TotalSamplePairs)
		for _, r := range rows {
			assert.Empty(t, r.Verdict)
		}

		keyFile, err := os.Open(res.KeyPath)
		require.NoError(t, err)
		defer func() { _ = keyFile.Close() }()
		key, err := ReadKeyJSON(keyFile)
		require.NoError(t, err)
		assert.Equal(t, "skipped", key.Representations["summary_vector"])
		require.Len(t, key.Pairs, TotalSamplePairs)
		for _, p := range key.Pairs {
			_, hasCM := p.Similarities["chunk_mean"]
			_, hasFC := p.Similarities["first_chunk"]
			_, hasNP := p.Similarities["nearest_chunk_pair"]
			_, hasSV := p.Similarities["summary_vector"]
			assert.True(t, hasCM)
			assert.True(t, hasFC)
			assert.True(t, hasNP)
			assert.False(t, hasSV)
		}
	})

	t.Run("with summaries embeds and records summary_vector", func(t *testing.T) {
		outDirSum := filepath.Join(t.TempDir(), "out")
		summaries := make(map[string]string, count)
		for i, id := range readIDs {
			summaries[id] = "summary text " + id + string(make([]byte, i%10))
		}
		encoder := &fakeSummaryEncoder{}
		runner := NewSampleRunner(uc, metaPort, encoder)
		res, err := runner.Run(context.Background(), SampleOptions{
			UserID:    uuid.NewString(),
			ReadIDs:   readIDs,
			OutDir:    outDirSum,
			Seed:      123,
			Summaries: summaries,
		})
		require.NoError(t, err)
		assert.Greater(t, encoder.calls, 0)

		keyFile, err := os.Open(res.KeyPath)
		require.NoError(t, err)
		defer func() { _ = keyFile.Close() }()
		key, err := ReadKeyJSON(keyFile)
		require.NoError(t, err)
		assert.Equal(t, "available", key.Representations["summary_vector"])
		for _, p := range key.Pairs {
			_, hasSV := p.Similarities["summary_vector"]
			assert.True(t, hasSV)
		}
	})
}
