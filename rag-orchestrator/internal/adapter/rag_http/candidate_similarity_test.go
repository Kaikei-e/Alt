package rag_http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"rag-orchestrator/internal/adapter/rag_http"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubCandidateSimilarityUsecase struct {
	called bool
	input  usecase.CandidateSimilarityInput
	output *usecase.CandidateSimilarityOutput
	err    error
}

func (s *stubCandidateSimilarityUsecase) Execute(_ context.Context, input usecase.CandidateSimilarityInput) (*usecase.CandidateSimilarityOutput, error) {
	s.called = true
	s.input = input
	if s.err != nil {
		return nil, s.err
	}
	return s.output, nil
}

func TestCandidateSimilarity_Success(t *testing.T) {
	e := echo.New()
	stub := &stubCandidateSimilarityUsecase{
		output: &usecase.CandidateSimilarityOutput{
			SeedIndexed:                true,
			CandidatesWithoutEmbedding: 1,
			Results: []domain.CandidateSimilarityItem{
				{ArticleID: "cand-1", Similarity: 0.92},
				{ArticleID: "cand-2", Similarity: 0.55},
			},
		},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := rag_http.NewHandler(nil, nil, nil, nil, nil, logger, rag_http.WithCandidateSimilarity(stub))

	body := bytes.NewBufferString(`{"seed_article_id":"seed-1","candidate_article_ids":["cand-1","cand-2","cand-3"],"representation":"chunk_mean","user_id":"` + testUserID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/rag/candidate-similarity", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.CandidateSimilarity(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, stub.called)
	assert.Equal(t, "seed-1", stub.input.SeedArticleID)
	assert.Equal(t, []string{"cand-1", "cand-2", "cand-3"}, stub.input.CandidateArticleIDs)
	assert.Equal(t, "chunk_mean", stub.input.Representation)
	assert.Equal(t, testUserID, stub.input.UserID)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["seed_indexed"])
	assert.Equal(t, float64(1), resp["candidates_without_embedding"])
	results, ok := resp["results"].([]any)
	require.True(t, ok)
	require.Len(t, results, 2)
}

func TestCandidateSimilarity_AcceptsUserIDHeaderAndExplicitFalseFlag(t *testing.T) {
	e := echo.New()
	stub := &stubCandidateSimilarityUsecase{
		output: &usecase.CandidateSimilarityOutput{
			SeedIndexed:                false,
			CandidatesWithoutEmbedding: 0,
			Results:                    []domain.CandidateSimilarityItem{},
		},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := rag_http.NewHandler(nil, nil, nil, nil, nil, logger, rag_http.WithCandidateSimilarity(stub))

	body := bytes.NewBufferString(`{"seed_article_id":"seed-1","candidate_article_ids":["cand-1"],"representation":"first_chunk"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/rag/candidate-similarity", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Alt-User-Id", testUserID)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.CandidateSimilarity(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, testUserID, stub.input.UserID)
	assert.Contains(t, rec.Body.String(), `"seed_indexed":false`)
	assert.Contains(t, rec.Body.String(), `"candidates_without_embedding":0`)
	assert.Contains(t, rec.Body.String(), `"results":[]`)
}

func TestCandidateSimilarity_ValidationErrorsReturn400(t *testing.T) {
	e := echo.New()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	oversize := make([]string, domain.MaxCandidateSimilarityArticles+1)
	for i := range oversize {
		oversize[i] = fmt.Sprintf("cand-%d", i)
	}
	oversizeJSON, _ := json.Marshal(map[string]any{
		"seed_article_id":       "seed-1",
		"candidate_article_ids": oversize,
		"representation":        "first_chunk",
		"user_id":               testUserID,
	})

	tests := []struct {
		name string
		body string
		uc   usecase.CandidateSimilarityUsecase
	}{
		{
			name: "invalid json",
			body: `{not-json`,
			uc:   &stubCandidateSimilarityUsecase{},
		},
		{
			name: "missing user_id",
			body: `{"seed_article_id":"seed-1","candidate_article_ids":["cand-1"],"representation":"first_chunk"}`,
			uc:   &stubCandidateSimilarityUsecase{},
		},
		{
			name: "nil uuid user_id",
			body: `{"seed_article_id":"seed-1","candidate_article_ids":["cand-1"],"representation":"first_chunk","user_id":"00000000-0000-0000-0000-000000000000"}`,
			uc:   &stubCandidateSimilarityUsecase{},
		},
		{
			name: "missing seed_article_id",
			body: `{"seed_article_id":"","candidate_article_ids":["cand-1"],"representation":"first_chunk","user_id":"` + testUserID + `"}`,
			uc:   &stubCandidateSimilarityUsecase{},
		},
		{
			name: "invalid representation",
			body: `{"seed_article_id":"seed-1","candidate_article_ids":["cand-1"],"representation":"bad","user_id":"` + testUserID + `"}`,
			uc:   &stubCandidateSimilarityUsecase{},
		},
		{
			name: "too many candidates",
			body: string(oversizeJSON),
			uc:   usecase.NewCandidateSimilarityUsecase(nil),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := rag_http.NewHandler(nil, nil, nil, nil, nil, logger, rag_http.WithCandidateSimilarity(tc.uc))
			req := httptest.NewRequest(http.MethodPost, "/internal/rag/candidate-similarity", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			require.NoError(t, handler.CandidateSimilarity(c))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestCandidateSimilarity_InternalErrorHidesDetails(t *testing.T) {
	e := echo.New()
	stub := &stubCandidateSimilarityUsecase{
		err: errors.New("pq: secret internal sql error"),
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := rag_http.NewHandler(nil, nil, nil, nil, nil, logger, rag_http.WithCandidateSimilarity(stub))

	body := bytes.NewBufferString(`{"seed_article_id":"seed-1","candidate_article_ids":["cand-1"],"representation":"first_chunk","user_id":"` + testUserID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/rag/candidate-similarity", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, handler.CandidateSimilarity(c))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "secret internal sql error")
}

func TestCandidateSimilarity_UnwiredUsecasePanics(t *testing.T) {
	e := echo.New()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := rag_http.NewHandler(nil, nil, nil, nil, nil, logger)

	body := bytes.NewBufferString(`{"seed_article_id":"seed-1","candidate_article_ids":["cand-1"],"representation":"first_chunk","user_id":"` + testUserID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/rag/candidate-similarity", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.Panics(t, func() {
		_ = handler.CandidateSimilarity(c)
	})
}
