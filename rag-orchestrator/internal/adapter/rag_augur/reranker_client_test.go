package rag_augur

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRerankerClient_Rerank_Success(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/rerank", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req RerankRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)

		assert.Equal(t, "test query", req.Query)
		assert.Equal(t, 3, len(req.Candidates))
		assert.Equal(t, "bge-reranker-v2-m3", req.Model)

		// Return reranked results (index 1 has highest score)
		resp := RerankResponse{
			Results: []RerankResponseResult{
				{Index: 1, Score: 0.95},
				{Index: 0, Score: 0.85},
				{Index: 2, Score: 0.75},
			},
			Model: "bge-reranker-v2-m3",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewRerankerClient(server.URL, "bge-reranker-v2-m3", 30*time.Second, logger)

	candidates := []domain.RerankCandidate{
		{ID: "chunk-1", Content: "Content about AI", Score: 0.8},
		{ID: "chunk-2", Content: "Content about machine learning", Score: 0.7},
		{ID: "chunk-3", Content: "Content about data science", Score: 0.6},
	}

	results, err := client.Rerank(context.Background(), "test query", candidates)
	require.NoError(t, err)

	assert.Len(t, results, 3)
	// Results should be in order returned by server (chunk-2 first with highest score)
	assert.Equal(t, "chunk-2", results[0].ID)
	assert.Equal(t, float32(0.95), results[0].Score)
	assert.Equal(t, "chunk-1", results[1].ID)
	assert.Equal(t, float32(0.85), results[1].Score)
	assert.Equal(t, "chunk-3", results[2].ID)
	assert.Equal(t, float32(0.75), results[2].Score)
}

func TestRerankerClient_Rerank_EmptyCandidates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewRerankerClient("http://localhost:8001", "bge-reranker-v2-m3", 30*time.Second, logger)

	results, err := client.Rerank(context.Background(), "test query", []domain.RerankCandidate{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestRerankerClient_Rerank_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewRerankerClient(server.URL, "bge-reranker-v2-m3", 30*time.Second, logger)

	candidates := []domain.RerankCandidate{
		{ID: "chunk-1", Content: "Content about AI", Score: 0.8},
	}

	results, err := client.Rerank(context.Background(), "test query", candidates)
	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "500")
}

func TestRerankerClient_Rerank_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Delay longer than timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewRerankerClient(server.URL, "bge-reranker-v2-m3", 10*time.Millisecond, logger)

	candidates := []domain.RerankCandidate{
		{ID: "chunk-1", Content: "Content about AI", Score: 0.8},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	results, err := client.Rerank(ctx, "test query", candidates)
	assert.Error(t, err)
	assert.Nil(t, results)
}

func TestRerankerClient_Rerank_InvalidIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RerankResponse{
			Results: []RerankResponseResult{
				{Index: 99, Score: 0.95}, // Invalid index
			},
			Model: "bge-reranker-v2-m3",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewRerankerClient(server.URL, "bge-reranker-v2-m3", 30*time.Second, logger)

	candidates := []domain.RerankCandidate{
		{ID: "chunk-1", Content: "Content about AI", Score: 0.8},
	}

	results, err := client.Rerank(context.Background(), "test query", candidates)
	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Contains(t, err.Error(), "invalid result index")
}

func TestRerankerClient_ModelName(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	client := NewRerankerClient("http://localhost:8001", "bge-reranker-v2-m3", 30*time.Second, logger)

	assert.Equal(t, "bge-reranker-v2-m3", client.ModelName())
}
