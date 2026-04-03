//go:build contract

// Package contract contains Consumer-Driven Contract tests for rag-orchestrator → news-creator.
//
// These tests verify the /api/chat proxy contract, specifically:
// - PM-006: chat requests must go through news-creator (not directly to Ollama)
// - PM-008: options must match batch summarization options to prevent model reload
package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pactDir = "../../../pacts"

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}

type chatStreamChunk struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

func newNewsCreatorPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "rag-orchestrator",
		Provider: "news-creator",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

// TestChatStreamContract verifies the streaming chat endpoint contract.
// This is the endpoint that PM-006 required to go through news-creator's semaphore.
// Note: Pact V3 doesn't natively support NDJSON streaming, so we verify the
// request contract (model, messages, stream=true) and response status/content-type.
func TestChatStreamContract(t *testing.T) {
	mockProvider := newNewsCreatorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the LLM model is loaded and ready for chat").
		UponReceiving("a streaming chat request from rag-orchestrator").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/api/chat"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"model":    matchers.Like("gemma4-e4b-q4km"),
				"messages": matchers.Like([]chatMessage{{Role: "user", Content: "test"}}),
				"stream":   matchers.Like(true),
				"options":  matchers.Like(map[string]any{"num_predict": 2048}),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/x-ndjson"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			apiURL := fmt.Sprintf("http://%s:%d/api/chat", config.Host, config.Port)

			payload := chatRequest{
				Model:    "gemma4-e4b-q4km",
				Messages: []chatMessage{{Role: "user", Content: "test"}},
				Stream:   true,
				Options:  map[string]any{"num_predict": 2048},
			}
			jsonData, _ := json.Marshal(payload)

			req, _ := http.NewRequestWithContext(context.Background(), "POST", apiURL, strings.NewReader(string(jsonData)))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("chat request failed: %w", err)
			}
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Contains(t, resp.Header.Get("Content-Type"), "application/x-ndjson")
			return nil
		})
	require.NoError(t, err)
}

// TestChatNonStreamContract verifies the non-streaming chat endpoint.
// Added per PM-008 fix: Morning Letter support.
func TestChatNonStreamContract(t *testing.T) {
	mockProvider := newNewsCreatorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the LLM model is loaded and ready for chat").
		UponReceiving("a non-streaming chat request from rag-orchestrator").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/api/chat"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"model":    matchers.Like("gemma4-e4b-q4km"),
				"messages": matchers.Like([]chatMessage{{Role: "user", Content: "test"}}),
				"stream":   matchers.Like(false),
				"options":  matchers.Like(map[string]any{"num_predict": 2048}),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"message": matchers.Like(map[string]interface{}{
					"content": "This is a non-streaming response.",
				}),
				"done": matchers.Like(true),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			apiURL := fmt.Sprintf("http://%s:%d/api/chat", config.Host, config.Port)

			payload := chatRequest{
				Model:    "gemma4-e4b-q4km",
				Messages: []chatMessage{{Role: "user", Content: "test"}},
				Stream:   false,
				Options:  map[string]any{"num_predict": 2048},
			}
			jsonData, _ := json.Marshal(payload)

			req, _ := http.NewRequestWithContext(context.Background(), "POST", apiURL, strings.NewReader(string(jsonData)))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("chat request failed: %w", err)
			}
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var chatResp chatStreamChunk
			if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			assert.NotEmpty(t, chatResp.Message.Content)
			assert.True(t, chatResp.Done)
			return nil
		})
	require.NoError(t, err)
}
