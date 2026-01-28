package rag_augur

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"rag-orchestrator/internal/domain"
	"time"
)

type OllamaEmbedder struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewOllamaEmbedder(baseURL, model string, timeoutSeconds int) *OllamaEmbedder {
	timeout := 30 * time.Second
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	return &OllamaEmbedder{
		BaseURL: baseURL,
		Model:   model,
		Client:  &http.Client{Timeout: timeout},
	}
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

func (e *OllamaEmbedder) Encode(ctx context.Context, texts []string) ([][]float32, error) {
	slog.Info("ollama_embed_started",
		slog.Int("text_count", len(texts)),
		slog.String("model", e.Model),
		slog.String("url", e.BaseURL),
	)
	start := time.Now()

	reqBody := embedRequest{
		Model: e.Model,
		Input: texts,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/embed", e.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.Client.Do(req)
	if err != nil {
		slog.Error("ollama_embed_failed",
			slog.String("error", err.Error()),
			slog.Duration("elapsed", time.Since(start)),
		)
		return nil, fmt.Errorf("failed to call ollama: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Error("ollama_embed_bad_status",
			slog.Int("status", resp.StatusCode),
			slog.Duration("elapsed", time.Since(start)),
		)
		return nil, fmt.Errorf("ollama returned status: %d", resp.StatusCode)
	}

	var respBody embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	slog.Info("ollama_embed_completed",
		slog.Int("embedding_count", len(respBody.Embeddings)),
		slog.Duration("elapsed", time.Since(start)),
	)

	return respBody.Embeddings, nil
}

func (e *OllamaEmbedder) Version() string {
	return e.Model
}

var _ domain.VectorEncoder = (*OllamaEmbedder)(nil)
