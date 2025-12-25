package rag_augur

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"rag-orchestrator/internal/domain"
)

const (
	generationTemperature = 0.0
	keepAliveSeconds      = 600
)

var generationFormat = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"quotes": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"chunk_id": map[string]interface{}{"type": "string"},
					"quote":    map[string]interface{}{"type": "string"},
				},
				"required": []string{"chunk_id", "quote"},
			},
		},
		"answer": map[string]interface{}{
			"type": "string",
		},
		"citations": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"chunk_id": map[string]interface{}{"type": "string"},
					"url":      map[string]interface{}{"type": "string"},
					"title":    map[string]interface{}{"type": "string"},
					"score":    map[string]interface{}{"type": "number"},
				},
				"required": []string{"chunk_id"},
			},
		},
		"fallback": map[string]interface{}{
			"type": "boolean",
		},
		"reason": map[string]interface{}{
			"type": "string",
		},
	},
	"required": []string{"quotes", "answer", "citations", "fallback", "reason"},
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string                 `json:"model"`
	Messages  []chatMessage          `json:"messages"`
	KeepAlive int                    `json:"keep_alive"`
	Format    map[string]interface{} `json:"format"`
	Options   map[string]interface{} `json:"options,omitempty"`
	MaxTokens *int                   `json:"max_tokens,omitempty"`
	Think     string                 `json:"think,omitempty"`
}

type chatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// OllamaGenerator sends prompts to Ollama's chat endpoint and returns structured text.
type OllamaGenerator struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

// NewOllamaGenerator constructs a generator using the provided endpoint and model name.
func NewOllamaGenerator(baseURL, model string) *OllamaGenerator {
	return &OllamaGenerator{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		Client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Generate sends the prompt to Ollama and returns the assistant message.
func (g *OllamaGenerator) Generate(ctx context.Context, prompt string, maxTokens int) (*domain.LLMResponse, error) {
	var maxTokensPtr *int = nil
	// Unused now as we use Options["num_predict"]
	_ = maxTokensPtr

	reqBody := chatRequest{
		Model:     g.Model,
		Messages:  []chatMessage{{Role: "user", Content: prompt}},
		KeepAlive: -1,
		Format:    generationFormat,
		Think:     "low",
		Options: map[string]interface{}{
			"temperature": 0.2,
		},
	}
	if maxTokens > 0 {
		reqBody.Options["num_predict"] = maxTokens
	}

	jsonPayload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", g.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call generation endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("generation endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode generation response: %w", err)
	}

	content := strings.TrimSpace(chatResp.Message.Content)

	return &domain.LLMResponse{
		Text: content,
		Done: chatResp.Done,
	}, nil
}

// Version returns the wrapped model name.
func (g *OllamaGenerator) Version() string {
	return g.Model
}

var _ domain.LLMClient = (*OllamaGenerator)(nil)
