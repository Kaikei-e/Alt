package rag_augur

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
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
	logger  *slog.Logger
}

// NewOllamaEmbedder constructs an embedder.
// If client is nil, a default http.Client is created with the given timeout.
// If logger is nil, slog.Default() is used.
func NewOllamaEmbedder(baseURL, model string, timeoutSeconds int, logger *slog.Logger, client ...*http.Client) *OllamaEmbedder {
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
	if logger == nil {
		logger = slog.Default()
	}
	return &OllamaEmbedder{
		BaseURL: baseURL,
		Model:   model,
		Client:  c,
		logger:  logger,
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
	e.logger.Info("ollama_embed_started",
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
		e.logger.Error("ollama_embed_failed",
			slog.String("category", category),
			slog.String("error", err.Error()),
			slog.Duration("elapsed", time.Since(start)),
		)
		return nil, fmt.Errorf("failed to call ollama (%s): %w", category, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		e.logger.Error("ollama_embed_bad_status",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(body)),
			slog.Duration("elapsed", time.Since(start)),
		)
		return nil, fmt.Errorf("ollama returned status: %d", resp.StatusCode)
	}

	var respBody embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		e.logger.Error("ollama_embed_decode_failed",
			slog.String("error", err.Error()),
			slog.String("category", "decode_failure"),
			slog.Duration("elapsed", time.Since(start)),
		)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if err := e.rejectDegenerate(respBody.Embeddings, len(texts), start); err != nil {
		return nil, err
	}

	e.logger.Info("ollama_embed_completed",
		slog.Int("embedding_count", len(respBody.Embeddings)),
		slog.Duration("elapsed", time.Since(start)),
	)

	return respBody.Embeddings, nil
}

// ErrDegenerateEmbedding marks a response that was structurally valid but
// carries no information. Callers must treat it as a hard failure: an
// indexing job that persists such a vector cannot be detected afterwards.
var ErrDegenerateEmbedding = errors.New("degenerate embedding")

// rejectDegenerate refuses all-zero, non-finite and empty vectors.
//
// Ollama has an open defect (ollama/ollama#17878) where /api/embed under
// sustained load answers 200 with correctly shaped, entirely zero vectors and
// ordinary usage stats. Cosine distance to a zero vector is undefined and
// pgvector scores it 0 against everything, so a document embedded during such
// a window is silently unretrievable for the life of its version — and no
// later query, log line or metric can tell it apart from a document nobody
// asked for. There is no safe degraded mode here: the batch has to fail so the
// job fails with it.
func (e *OllamaEmbedder) rejectDegenerate(vectors [][]float32, batchSize int, start time.Time) error {
	for i, vec := range vectors {
		reason := degenerateReason(vec)
		if reason == "" {
			continue
		}
		e.logger.Error("ollama_embed_degenerate_vector",
			slog.String("reason", reason),
			slog.Int("index", i),
			slog.Int("batch_size", batchSize),
			slog.Int("returned_count", len(vectors)),
			slog.Int("dimension", len(vec)),
			slog.String("model", e.Model),
			slog.String("url", e.BaseURL),
			slog.Duration("elapsed", time.Since(start)),
		)
		return fmt.Errorf("%w: vector %d of %d is %s", ErrDegenerateEmbedding, i, len(vectors), reason)
	}
	return nil
}

// degenerateReason names why a vector is unusable, or "" when it is fine.
func degenerateReason(vec []float32) string {
	if len(vec) == 0 {
		return "empty"
	}
	allZero := true
	for _, c := range vec {
		f := float64(c)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return "non_finite"
		}
		if c != 0 {
			allZero = false
		}
	}
	if allZero {
		return "all_zero"
	}
	return ""
}

func (e *OllamaEmbedder) Version() string {
	return domain.EmbedderVersion(e.Model)
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
