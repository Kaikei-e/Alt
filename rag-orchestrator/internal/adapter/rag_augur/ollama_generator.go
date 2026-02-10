package rag_augur

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
)

var generationFormat = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"answer": map[string]interface{}{
			"type": "string",
		},
		"citations": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"chunk_id": map[string]interface{}{"type": "string"},
					"reason":   map[string]interface{}{"type": "string"},
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
	"required": []string{"answer", "citations", "fallback", "reason"},
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
	Think     interface{}            `json:"think,omitempty"`
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
	logger  *slog.Logger
}

// NewOllamaGenerator constructs a generator using the provided endpoint and model name.
// If client is nil, a default http.Client is created with the given timeout.
func NewOllamaGenerator(baseURL, model string, timeout int, logger *slog.Logger, client ...*http.Client) *OllamaGenerator {
	var c *http.Client
	if len(client) > 0 && client[0] != nil {
		c = client[0]
	} else {
		c = &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		}
	}
	return &OllamaGenerator{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		Client:  c,
		logger:  logger,
	}
}

// getThinkParam returns the appropriate think parameter for the model.
// qwen3 models require boolean false to disable thinking.
// swallow/llama models: thinking not supported, return nil.
// gpt-oss models use string levels ("low", "medium").
func (g *OllamaGenerator) getThinkParam(maxTokens int) interface{} {
	modelLower := strings.ToLower(g.Model)
	// qwen3 models: disable thinking with boolean false
	if strings.Contains(modelLower, "qwen3") {
		return false
	}
	// swallow/llama models: do not support thinking, skip parameter
	if strings.Contains(modelLower, "swallow") || strings.Contains(modelLower, "llama") {
		return nil
	}
	// gemma models: thinking not supported, return nil
	if strings.Contains(modelLower, "gemma") {
		return nil
	}
	// gpt-oss and other models: use string levels
	// Short tasks (maxTokens < 300) use "low" - e.g., query expansion
	// Longer tasks use "medium" - e.g., knowledge synthesis
	if maxTokens > 0 && maxTokens < 300 {
		return "low"
	}
	return "medium"
}

// buildOptions returns the appropriate options map for the model.
// Different models require different sampling parameters.
func (g *OllamaGenerator) buildOptions(maxTokens int) map[string]interface{} {
	opts := map[string]interface{}{}
	modelLower := strings.ToLower(g.Model)

	switch {
	case strings.Contains(modelLower, "qwen3"):
		// Qwen3: 繰り返し防止パラメータ（公式推奨値）
		opts["temperature"] = 0.7
		opts["top_p"] = 0.8
		opts["repeat_penalty"] = 1.15
	case strings.Contains(modelLower, "gemma"):
		// Gemma 3: news-creator実績準拠の設定
		// M4 Mac Mini 16GB: 12B Q4_0 ~7GB + 8K KV cache ~2GB = ~9GB
		opts["temperature"] = 0.7
		opts["top_p"] = 0.85
		opts["top_k"] = 40
		opts["num_ctx"] = 8192
		opts["repeat_penalty"] = 1.15
	case strings.Contains(modelLower, "swallow") || strings.Contains(modelLower, "llama"):
		// Swallow/Llama 3.1: 詳細な回答生成向け設定
		// Swallow公式推奨: temperature=0.6 for detailed responses
		// Reference: https://swallow-llm.github.io/llama3.1-swallow.en.html
		opts["temperature"] = 0.6
		opts["top_p"] = 0.9
		// コンテキストウィンドウ: RAMで swallow-8b q5_k_s (~6GB) + 16k KV cache (~1.5GB) = ~7.5GB
		opts["num_ctx"] = 16384
		// 長い回答のためのrepeat_penalty（繰り返し防止）
		opts["repeat_penalty"] = 1.1
	default:
		// gpt-oss and other models
		opts["temperature"] = 0.2
	}

	if maxTokens > 0 {
		opts["num_predict"] = maxTokens
	}

	return opts
}

// Generate sends the prompt to Ollama and returns the assistant message.
func (g *OllamaGenerator) Generate(ctx context.Context, prompt string, maxTokens int) (*domain.LLMResponse, error) {
	requestID := uuid.NewString()
	g.logger.Info("ollama_generate_started",
		slog.String("request_id", requestID),
		slog.String("model", g.Model),
		slog.Int("max_tokens", maxTokens),
		slog.Int("prompt_length", len(prompt)))

	var maxTokensPtr *int = nil
	// Unused now as we use Options["num_predict"]
	_ = maxTokensPtr

	opts := g.buildOptions(maxTokens)

	reqBody := chatRequest{
		Model:     g.Model,
		Messages:  []chatMessage{{Role: "user", Content: prompt}},
		KeepAlive: -1,
		Stream:    true,
		// Format:    nil, // Force generic
		Think:   g.getThinkParam(maxTokens),
		Options: opts,
	}

	jsonPayload, err := json.Marshal(reqBody)
	if err != nil {
		g.logger.Warn("ollama_generate_failed",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", g.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonPayload))
	if err != nil {
		g.logger.Warn("ollama_generate_failed",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	g.logger.Info("ollama_request_sent",
		slog.String("request_id", requestID),
		slog.String("url", url))

	resp, err := g.Client.Do(req)
	if err != nil {
		g.logger.Warn("ollama_generate_failed",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to call generation endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		g.logger.Warn("ollama_generate_failed",
			slog.String("request_id", requestID),
			slog.String("error", fmt.Sprintf("status %d: %s", resp.StatusCode, string(body))))
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
			g.logger.Warn("ollama_generate_failed",
				slog.String("request_id", requestID),
				slog.String("error", err.Error()))
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
		g.logger.Warn("ollama_generate_failed",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to read generation stream: %w", err)
	}

	content := strings.TrimSpace(builder.String())

	g.logger.Info("ollama_generate_completed",
		slog.String("request_id", requestID),
		slog.Int("response_length", len(content)),
		slog.Bool("done", done))

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

	opts := g.buildOptions(maxTokens)

	reqBody := chatRequest{
		Model:     g.Model,
		Messages:  []chatMessage{{Role: "user", Content: prompt}},
		KeepAlive: -1,
		Stream:    true,
		Think:     g.getThinkParam(maxTokens),
		Options:   opts,
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
		_ = resp.Body.Close()
		return nil, nil, fmt.Errorf("stream endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	chunkCh := make(chan domain.LLMStreamChunk)
	errCh := make(chan error, 1)

	go func() {
		defer func() { _ = resp.Body.Close() }()
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
			var thinking string
			if msg, ok := raw["message"].(map[string]interface{}); ok {
				content = toString(msg["content"])
				thinking = toString(msg["thinking"])
			} else if response, ok := raw["response"]; ok {
				// Fallback to legacy format if needed
				content = toString(response)
			}

			chunk := domain.LLMStreamChunk{
				Response:   content,
				Thinking:   thinking,
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

// Chat sends a conversation history to Ollama and returns the assistant message.
func (g *OllamaGenerator) Chat(ctx context.Context, messages []domain.Message, maxTokens int) (*domain.LLMResponse, error) {
	requestID := uuid.NewString()
	g.logger.Info("ollama_chat_started",
		slog.String("request_id", requestID),
		slog.String("model", g.Model),
		slog.Int("message_count", len(messages)),
		slog.Int("max_tokens", maxTokens))

	chatMsgs := toChatMessages(messages)
	opts := g.buildOptions(maxTokens)

	reqBody := chatRequest{
		Model:     g.Model,
		Messages:  chatMsgs,
		KeepAlive: -1,
		Stream:    false,
		Format:    generationFormat,
		Think:     g.getThinkParam(maxTokens),
		Options:   opts,
	}

	jsonPayload, err := json.Marshal(reqBody)
	if err != nil {
		g.logger.Warn("ollama_chat_failed",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", g.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonPayload))
	if err != nil {
		g.logger.Warn("ollama_chat_failed",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	g.logger.Info("ollama_request_sent",
		slog.String("request_id", requestID),
		slog.String("url", url))

	resp, err := g.Client.Do(req)
	if err != nil {
		g.logger.Warn("ollama_chat_failed",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to call chat endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		g.logger.Warn("ollama_chat_failed",
			slog.String("request_id", requestID),
			slog.String("error", fmt.Sprintf("status %d: %s", resp.StatusCode, string(body))))
		return nil, fmt.Errorf("chat endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		g.logger.Warn("ollama_chat_failed",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to decode chat response: %w", err)
	}

	g.logger.Info("ollama_chat_completed",
		slog.String("request_id", requestID),
		slog.Int("response_length", len(chatResp.Message.Content)),
		slog.Bool("done", chatResp.Done))

	return &domain.LLMResponse{
		Text: chatResp.Message.Content,
		Done: chatResp.Done,
	}, nil
}

// ChatStream streams conversation history to Ollama.
func (g *OllamaGenerator) ChatStream(ctx context.Context, messages []domain.Message, maxTokens int) (<-chan domain.LLMStreamChunk, <-chan error, error) {
	requestID := uuid.NewString()
	g.logger.Info("ollama_chat_stream_started",
		slog.String("request_id", requestID),
		slog.String("model", g.Model),
		slog.Int("message_count", len(messages)),
		slog.Int("max_tokens", maxTokens))

	if len(messages) == 0 {
		return nil, nil, fmt.Errorf("messages are required for streaming chat")
	}

	chatMsgs := toChatMessages(messages)
	opts := g.buildOptions(maxTokens)

	// Swallow/Llama/Gemma models support structured output via Format
	// gpt-oss: Format causes empty content, skip
	var format map[string]interface{}
	modelLower := strings.ToLower(g.Model)
	if strings.Contains(modelLower, "swallow") || strings.Contains(modelLower, "llama") || strings.Contains(modelLower, "gemma") {
		format = generationFormat
	}

	reqBody := chatRequest{
		Model:     g.Model,
		Messages:  chatMsgs,
		KeepAlive: -1,
		Stream:    true,
		Format:    format,
		Think:     g.getThinkParam(maxTokens),
		Options:   opts,
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

	g.logger.Info("ollama_chat_stream_request_sent",
		slog.String("request_id", requestID),
		slog.String("url", url))

	resp, err := g.Client.Do(req)
	if err != nil {
		g.logger.Warn("ollama_chat_stream_failed",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()))
		return nil, nil, fmt.Errorf("failed to call stream endpoint: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		g.logger.Warn("ollama_chat_stream_failed",
			slog.String("request_id", requestID),
			slog.String("error", fmt.Sprintf("status %d: %s", resp.StatusCode, string(body))))
		return nil, nil, fmt.Errorf("stream endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	g.logger.Info("ollama_chat_stream_connected",
		slog.String("request_id", requestID))

	chunkCh := make(chan domain.LLMStreamChunk)
	errCh := make(chan error, 1)

	go func() {
		defer func() { _ = resp.Body.Close() }()
		defer close(chunkCh)
		defer close(errCh)

		chunkCount := 0
		totalBytes := 0
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				g.logger.Warn("ollama_chat_stream_cancelled",
					slog.String("request_id", requestID),
					slog.Int("chunks_received", chunkCount))
				return
			default:
			}

			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var raw map[string]interface{}
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				g.logger.Warn("ollama_chat_stream_decode_error",
					slog.String("request_id", requestID),
					slog.String("error", err.Error()),
					slog.String("line_preview", truncateString(line, 200)))
				errCh <- fmt.Errorf("failed to decode stream chunk: %w", err)
				return
			}

			// Log first chunk structure for debugging
			if chunkCount == 0 {
				g.logger.Info("ollama_chat_stream_first_chunk",
					slog.String("request_id", requestID),
					slog.String("raw_preview", truncateString(line, 500)))
			}

			var content string
			var thinking string
			if msg, ok := raw["message"].(map[string]interface{}); ok {
				// Extract content and thinking separately
				content = toString(msg["content"])
				thinking = toString(msg["thinking"])
			} else if response, ok := raw["response"]; ok {
				content = toString(response)
			}

			// gpt-oss workaround: model outputs to thinking instead of content
			// Use thinking as content when content is empty
			effectiveContent := content
			if effectiveContent == "" && thinking != "" {
				effectiveContent = thinking
				thinking = "" // Don't duplicate in Thinking field
			}

			chunkCount++
			totalBytes += len(effectiveContent) + len(thinking)

			chunk := domain.LLMStreamChunk{
				Response:   effectiveContent,
				Thinking:   thinking,
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
				g.logger.Warn("ollama_chat_stream_cancelled",
					slog.String("request_id", requestID),
					slog.Int("chunks_received", chunkCount))
				return
			case chunkCh <- chunk:
			}

			if chunk.Done {
				g.logger.Info("ollama_chat_stream_completed",
					slog.String("request_id", requestID),
					slog.Int("chunks_received", chunkCount),
					slog.Int("total_bytes", totalBytes),
					slog.String("done_reason", chunk.DoneReason))
				return
			}
		}

		if err := scanner.Err(); err != nil {
			g.logger.Warn("ollama_chat_stream_read_error",
				slog.String("request_id", requestID),
				slog.String("error", err.Error()),
				slog.Int("chunks_received", chunkCount))
			errCh <- fmt.Errorf("failed to read stream: %w", err)
			return
		}

		// Stream ended without an explicit done flag.
		// If we received substantial data, treat as success (done flag was just missing).
		// If minimal data, report as error so caller can fallback.
		if totalBytes < 50 {
			g.logger.Warn("ollama_chat_stream_ended_without_done_minimal_data",
				slog.String("request_id", requestID),
				slog.Int("chunks_received", chunkCount),
				slog.Int("total_bytes", totalBytes))
			errCh <- fmt.Errorf("stream ended without done flag and minimal data (%d bytes)", totalBytes)
		} else {
			g.logger.Warn("ollama_chat_stream_ended_without_done",
				slog.String("request_id", requestID),
				slog.Int("chunks_received", chunkCount),
				slog.Int("total_bytes", totalBytes))
		}
	}()

	return chunkCh, errCh, nil
}

func toChatMessages(msgs []domain.Message) []chatMessage {
	out := make([]chatMessage, len(msgs))
	for i, m := range msgs {
		out[i] = chatMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}
	return out
}

var _ domain.LLMClient = (*OllamaGenerator)(nil)
