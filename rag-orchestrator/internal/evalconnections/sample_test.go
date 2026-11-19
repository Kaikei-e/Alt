package evalconnections

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildSyntheticNeighbors(n int) (ids []string, chunkMean, firstChunk map[string][]domain.CandidateSimilarityItem) {
	ids = make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = fmt.Sprintf("art-%03d", i)
	}
	chunkMean = make(map[string][]domain.CandidateSimilarityItem, n)
	firstChunk = make(map[string][]domain.CandidateSimilarityItem, n)

	for i, seed := range ids {
		cm := make([]domain.CandidateSimilarityItem, 0, n-1)
		fc := make([]domain.CandidateSimilarityItem, 0, n-1)
		// Put different nearest neighbors in chunkMean vs firstChunk and populate >= 15 ranks.
		for offset := 1; offset < n; offset++ {
			candCM := ids[(i+offset)%n]
			cm = append(cm, domain.CandidateSimilarityItem{
				ArticleID:  candCM,
				Similarity: 0.99 - float64(offset)*0.01,
			})
			candFC := ids[(i+offset*3)%n]
			if candFC == seed {
				candFC = ids[(i+1)%n]
			}
			fc = append(fc, domain.CandidateSimilarityItem{
				ArticleID:  candFC,
				Similarity: 0.95 - float64(offset)*0.01,
			})
		}
		chunkMean[seed] = cm
		firstChunk[seed] = fc
	}
	return ids, chunkMean, firstChunk
}

func TestParseReadIDs(t *testing.T) {
	raw := "art-1\n\n  art-2  \n# comment\nart-1\nart-3\n"
	got, err := ParseReadIDs(strings.NewReader(raw))
	require.NoError(t, err)
	assert.Equal(t, []string{"art-1", "art-2", "art-3"}, got)
}

func TestParseTitlesAndSummariesTSV(t *testing.T) {
	titlesTSV := "art-1\tTitle 1\thttps://example.com/1\nart-2\tTitle 2\n"
	titles, err := ParseTitlesTSV(strings.NewReader(titlesTSV))
	require.NoError(t, err)
	assert.Equal(t, domain.RagDocumentMetadata{ArticleID: "art-1", Title: "Title 1", URL: "https://example.com/1"}, titles["art-1"])
	assert.Equal(t, domain.RagDocumentMetadata{ArticleID: "art-2", Title: "Title 2", URL: ""}, titles["art-2"])

	summariesTSV := "art-1\tFirst article summary\nart-2\tSecond\tsummary with tab\n"
	summaries, err := ParseSummariesTSV(strings.NewReader(summariesTSV))
	require.NoError(t, err)
	assert.Equal(t, "First article summary", summaries["art-1"])
	assert.Equal(t, "Second\tsummary with tab", summaries["art-2"])
}

func TestSelectSamplePairs_StrataCountsNoDuplicatesDeterministic(t *testing.T) {
	ids, chunkMean, firstChunk := buildSyntheticNeighbors(60)

	pairs1, err := SelectSamplePairs(ids, chunkMean, firstChunk, 42)
	require.NoError(t, err)
	require.Len(t, pairs1, TotalSamplePairs)

	counts := map[Stratum]int{}
	seenKeys := make(map[PairKey]struct{}, len(pairs1))
	for _, p := range pairs1 {
		counts[p.Stratum]++
		key := CanonicalPairKey(p.ArticleA, p.ArticleB)
		_, dup := seenKeys[key]
		assert.False(t, dup, "duplicate unordered pair %v", key)
		assert.NotEqual(t, p.ArticleA, p.ArticleB)
		seenKeys[key] = struct{}{}
	}
	assert.Equal(t, TargetChunkMeanNN, counts[StratumChunkMeanNN])
	assert.Equal(t, TargetFirstChunkNN, counts[StratumFirstChunkNN])
	assert.Equal(t, TargetChunkMeanRank5To15, counts[StratumChunkMeanRank5To15])
	assert.Equal(t, TargetRandom, counts[StratumRandom])

	// Verify deterministic for same seed.
	pairs2, err := SelectSamplePairs(ids, chunkMean, firstChunk, 42)
	require.NoError(t, err)
	assert.Equal(t, pairs1, pairs2)

	// Verify different seed changes order/selection.
	pairs3, err := SelectSamplePairs(ids, chunkMean, firstChunk, 99)
	require.NoError(t, err)
	assert.NotEqual(t, pairs1, pairs3)
}

func TestSelectSamplePairs_Rank5To15Constraint(t *testing.T) {
	ids, chunkMean, firstChunk := buildSyntheticNeighbors(30)
	pairs, err := SelectSamplePairs(ids, chunkMean, firstChunk, 7)
	require.NoError(t, err)

	// Every rank_5_15 pair must correspond to index 4..14 (1-based rank 5..15) for at least one endpoint.
	for _, p := range pairs {
		if p.Stratum != StratumChunkMeanRank5To15 {
			continue
		}
		inRange := false
		for _, pair := range [][2]string{{p.ArticleA, p.ArticleB}, {p.ArticleB, p.ArticleA}} {
			nbs := chunkMean[pair[0]]
			for r := 4; r <= 14 && r < len(nbs); r++ {
				if nbs[r].ArticleID == pair[1] {
					inRange = true
					break
				}
			}
		}
		assert.True(t, inRange, "pair (%s, %s) was not in chunk_mean ranks 5..15", p.ArticleA, p.ArticleB)
	}
}

func TestSelectSamplePairs_ErrorsWhenTooFewPairs(t *testing.T) {
	ids, chunkMean, firstChunk := buildSyntheticNeighbors(8)
	_, err := SelectSamplePairs(ids, chunkMean, firstChunk, 1)
	require.Error(t, err)
}

func TestWriteScoringSheetCSV_BlindAndBlankVerdict(t *testing.T) {
	rows := []SheetRow{
		{
			PairNumber: 1,
			TitleA:     "Alpha",
			URLA:       "https://example.com/a",
			TitleB:     "Beta",
			URLB:       "https://example.com/b",
			Verdict:    "",
		},
	}

	var buf bytes.Buffer
	require.NoError(t, WriteScoringSheetCSV(&buf, rows))

	raw := buf.String()
	assert.NotContains(t, raw, "similarity")
	assert.NotContains(t, raw, "stratum")
	assert.NotContains(t, raw, "chunk_mean")

	r := csv.NewReader(strings.NewReader(raw))
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, []string{"pair_number", "title_a", "url_a", "title_b", "url_b", "verdict"}, records[0])
	assert.Equal(t, []string{"1", "Alpha", "https://example.com/a", "Beta", "https://example.com/b", ""}, records[1])
}

func TestCosineSimilarity(t *testing.T) {
	sim, err := CosineSimilarity([]float32{1, 0, 0}, []float32{1, 0, 0})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, sim, 1e-9)

	sim, err = CosineSimilarity([]float32{1, 0}, []float32{0, 1})
	require.NoError(t, err)
	assert.InDelta(t, 0.0, sim, 1e-9)

	_, err = CosineSimilarity([]float32{0, 0}, []float32{1, 0})
	require.Error(t, err)
}
