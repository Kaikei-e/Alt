package rag_augur

import (
	"bufio"
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
	Stream    bool                   `json:"stream"`
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

type streamGenerationRequest struct {
	Model     string                 `json:"model"`
	Prompt    string                 `json:"prompt"`
	Stream    bool                   `json:"stream"`
	KeepAlive int                    `json:"keep_alive"`
	Format    map[string]interface{} `json:"format"`
	Options   map[string]interface{} `json:"options,omitempty"`
	Think     string                 `json:"think,omitempty"`
}

// OllamaGenerator sends prompts to Ollama's chat endpoint and returns structured text.
type OllamaGenerator struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

// NewOllamaGenerator constructs a generator using the provided endpoint and model name.
func NewOllamaGenerator(baseURL, model string, timeout int) *OllamaGenerator {
	return &OllamaGenerator{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		Client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
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
		Stream:    true,
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

	var builder strings.Builder
	done := false

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var chatResp chatResponse
		if err := json.Unmarshal([]byte(line), &chatResp); err != nil {
			return nil, fmt.Errorf("failed to decode generation response: %w", err)
		}

		if chatResp.Message.Content != "" {
			builder.WriteString(chatResp.Message.Content)
		}
		if chatResp.Done {
			done = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read generation stream: %w", err)
	}

	content := strings.TrimSpace(builder.String())

	return &domain.LLMResponse{
		Text: content,
		Done: done,
	}, nil
}

// GenerateStream streams prompts to Ollama's /api/chat endpoint.
func (g *OllamaGenerator) GenerateStream(ctx context.Context, prompt string, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, nil, fmt.Errorf("prompt is required for streaming generation")
	}

	reqBody := chatRequest{
		Model:     g.Model,
		Messages:  []chatMessage{{Role: "user", Content: prompt}},
		KeepAlive: -1,
		Stream:    true,
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
		return nil, nil, fmt.Errorf("failed to marshal stream request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", g.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonPayload))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to call stream endpoint: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, nil, fmt.Errorf("stream endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	chunkCh := make(chan domain.LLMStreamChunk)
	errCh := make(chan error, 1)

	go func() {
		defer resp.Body.Close()
		defer close(chunkCh)
		defer close(errCh)

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var raw map[string]interface{}
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				errCh <- fmt.Errorf("failed to decode stream chunk: %w", err)
				return
			}

			// Parse chat response format
			var content string
			if msg, ok := raw["message"].(map[string]interface{}); ok {
				content = toString(msg["content"])
			} else if response, ok := raw["response"]; ok {
				// Fallback to legacy format if needed
				content = toString(response)
			}

			chunk := domain.LLMStreamChunk{
				Response:   content,
				Model:      toString(raw["model"]),
				Done:       toBool(raw["done"]),
				DoneReason: toString(raw["done_reason"]),
			}
			if val, ok := toInt(raw["prompt_eval_count"]); ok {
				chunk.PromptEvalCount = val
			}
			if val, ok := toInt(raw["eval_count"]); ok {
				chunk.EvalCount = val
			}
			if val, ok := toInt64(raw["total_duration"]); ok {
				chunk.TotalDuration = val
			}

			select {
			case <-ctx.Done():
				return
			case chunkCh <- chunk:
			}

			if chunk.Done {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("failed to read stream: %w", err)
			return
		}
	}()

	return chunkCh, errCh, nil
}

// Version returns the wrapped model name.
func (g *OllamaGenerator) Version() string {
	return g.Model
}

func toString(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func toBool(value interface{}) bool {
	if value == nil {
		return false
	}
	if b, ok := value.(bool); ok {
		return b
	}
	return false
}

func toInt(value interface{}) (*int, bool) {
	switch v := value.(type) {
	case float64:
		i := int(v)
		return &i, true
	case int:
		return &v, true
	case int64:
		i := int(v)
		return &i, true
	default:
		return nil, false
	}
}

func toInt64(value interface{}) (*int64, bool) {
	switch v := value.(type) {
	case float64:
		i := int64(v)
		return &i, true
	case int:
		i := int64(v)
		return &i, true
	case int64:
		return &v, true
	default:
		return nil, false
	}
}

var _ domain.LLMClient = (*OllamaGenerator)(nil)
