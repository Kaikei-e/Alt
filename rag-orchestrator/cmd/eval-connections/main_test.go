package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"rag-orchestrator/internal/evalconnections"

	"rag-orchestrator/internal/fsperm"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSampleCommand_FailFastValidation(t *testing.T) {
	tmp := t.TempDir()
	readIDsPath := filepath.Join(tmp, "read_ids.txt")
	require.NoError(t, os.WriteFile(readIDsPath, []byte("a1\na2\n"), fsperm.SecretFile))

	t.Run("missing user_id fails fast", func(t *testing.T) {
		t.Setenv("EVAL_USER_ID", "")
		t.Setenv("DB_PASSWORD", "secret")
		err := runSample(sampleFlags{
			readIDsPath: readIDsPath,
			outDir:      filepath.Join(tmp, "out"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user-id")
	})

	t.Run("missing DB password fails fast", func(t *testing.T) {
		t.Setenv("DB_PASSWORD", "")
		t.Setenv("DB_PASSWORD_FILE", "")
		err := runSample(sampleFlags{
			readIDsPath: readIDsPath,
			outDir:      filepath.Join(tmp, "out"),
			userID:      uuid.NewString(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DB_PASSWORD")
	})

	t.Run("summaries flag with missing embedder auth fails fast", func(t *testing.T) {
		t.Setenv("DB_PASSWORD", "secret")
		t.Setenv("INFERENCE_AUTH", "")
		t.Setenv("INFERENCE_SERVICE_TOKEN_FILE", "")
		sumPath := filepath.Join(tmp, "summaries.tsv")
		require.NoError(t, os.WriteFile(sumPath, []byte("a1\thello\n"), fsperm.SecretFile))

		err := runSample(sampleFlags{
			readIDsPath:   readIDsPath,
			outDir:        filepath.Join(tmp, "out"),
			userID:        uuid.NewString(),
			summariesPath: sumPath,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "INFERENCE_SERVICE_TOKEN_FILE")
	})
}

func TestScoreCommand_EmitsReportAndJSON(t *testing.T) {
	tmp := t.TempDir()
	sheetPath := filepath.Join(tmp, "sheet.csv")
	keyPath := filepath.Join(tmp, "key.json")

	sheetContent := "pair_number,title_a,url_a,title_b,url_b,verdict\n" +
		"1,A1,https://e.com/1,B1,https://e.com/2,connected\n" +
		"2,A2,https://e.com/3,B2,https://e.com/4,connected\n" +
		"3,A3,https://e.com/5,B3,https://e.com/6,unsure\n" +
		"4,A4,https://e.com/7,B4,https://e.com/8,not_connected\n" +
		"5,A5,https://e.com/9,B5,https://e.com/10,not_connected\n"
	require.NoError(t, os.WriteFile(sheetPath, []byte(sheetContent), fsperm.SecretFile))

	key := evalconnections.KeyFile{
		Seed:             1,
		ThresholdVersion: 1,
		Representations: map[string]string{
			"first_chunk":        "available",
			"chunk_mean":         "available",
			"nearest_chunk_pair": "available",
			"summary_vector":     "skipped",
		},
		Pairs: []evalconnections.KeyPair{
			{PairNumber: 1, ArticleIDA: "a1", ArticleIDB: "b1", Stratum: evalconnections.StratumChunkMeanNN, Similarities: map[string]float64{"first_chunk": 0.9, "chunk_mean": 0.9, "nearest_chunk_pair": 0.85}},
			{PairNumber: 2, ArticleIDA: "a2", ArticleIDB: "b2", Stratum: evalconnections.StratumChunkMeanNN, Similarities: map[string]float64{"first_chunk": 0.4, "chunk_mean": 0.8, "nearest_chunk_pair": 0.80}},
			{PairNumber: 3, ArticleIDA: "a3", ArticleIDB: "b3", Stratum: evalconnections.StratumFirstChunkNN, Similarities: map[string]float64{"first_chunk": 0.5, "chunk_mean": 0.7, "nearest_chunk_pair": 0.70}},
			{PairNumber: 4, ArticleIDA: "a4", ArticleIDB: "b4", Stratum: evalconnections.StratumRandom, Similarities: map[string]float64{"first_chunk": 0.8, "chunk_mean": 0.3, "nearest_chunk_pair": 0.30}},
			{PairNumber: 5, ArticleIDA: "a5", ArticleIDB: "b5", Stratum: evalconnections.StratumRandom, Similarities: map[string]float64{"first_chunk": 0.8, "chunk_mean": 0.2, "nearest_chunk_pair": 0.20}},
		},
	}
	keyBytes, err := json.Marshal(key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyPath, keyBytes, fsperm.SecretFile))

	var out bytes.Buffer
	jsonOutPath := filepath.Join(tmp, "report.json")
	err = runScore(scoreFlags{
		sheetPath:   sheetPath,
		keyPath:     keyPath,
		jsonOutPath: jsonOutPath,
	}, &out)
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "PASS")
	assert.Contains(t, output, "chunk_mean")
	assert.Contains(t, output, `"threshold_version": 1`)
	assert.Contains(t, output, `"summary_vector"`)
	assert.Contains(t, output, "skipped")

	// #nosec G304 -- path is built from t.TempDir in this test
	savedJSON, err := os.ReadFile(jsonOutPath)
	require.NoError(t, err)
	var decoded evalconnections.ScoreReport
	require.NoError(t, json.Unmarshal(savedJSON, &decoded))
	assert.True(t, decoded.Passed)
	assert.Equal(t, 1, decoded.ThresholdVersion)
	require.NotNil(t, decoded.Recommendation)
	assert.Equal(t, "chunk_mean", decoded.Recommendation.Representation)
}
