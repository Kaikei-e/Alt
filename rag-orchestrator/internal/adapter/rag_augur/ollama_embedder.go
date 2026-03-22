package rag_augur

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"rag-orchestrator/internal/domain"
	"strings"
	"time"
)

type OllamaEmbedder struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

// NewOllamaEmbedder constructs an embedder.
// If client is nil, a default http.Client is created with the given timeout.
func NewOllamaEmbedder(baseURL, model string, timeoutSeconds int, client ...*http.Client) *OllamaEmbedder {
	var c *http.Client
	if len(client) > 0 && client[0] != nil {
		c = client[0]
	} else {
		timeout := 30 * time.Second
		if timeoutSeconds > 0 {
			timeout = time.Duration(timeoutSeconds) * time.Second
		}
		c = &http.Client{Timeout: timeout}
	}
	return &OllamaEmbedder{
		BaseURL: baseURL,
		Model:   model,
		Client:  c,
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
		category := classifyTransportError(err)
		slog.Error("ollama_embed_failed",
			slog.String("category", category),
			slog.String("error", err.Error()),
			slog.Duration("elapsed", time.Since(start)),
		)
		return nil, fmt.Errorf("failed to call ollama (%s): %w", category, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		slog.Error("ollama_embed_bad_status",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(body)),
			slog.Duration("elapsed", time.Since(start)),
		)
		return nil, fmt.Errorf("ollama returned status: %d", resp.StatusCode)
	}

	var respBody embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		slog.Error("ollama_embed_decode_failed",
			slog.String("error", err.Error()),
			slog.String("category", "decode_failure"),
			slog.Duration("elapsed", time.Since(start)),
		)
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

// classifyTransportError categorizes a transport error for structured logging.
// Distinguishes caller context expiry from http.Client.Timeout by inspecting
// the "Client.Timeout" substring that Go's net/http injects.
func classifyTransportError(err error) string {
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return "connection_failed"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if strings.Contains(err.Error(), "Client.Timeout") {
			return "client_timeout"
		}
		return "context_deadline_exceeded"
	}
	return "transport_error"
}

var _ domain.VectorEncoder = (*OllamaEmbedder)(nil)
