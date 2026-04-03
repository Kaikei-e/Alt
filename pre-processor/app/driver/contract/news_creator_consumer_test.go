//go:build contract

// Package contract contains Consumer-Driven Contract tests for pre-processor → news-creator.
//
// These tests verify that pre-processor's expectations of the news-creator
// HTTP/REST API (/api/v1/summarize) are documented as Pact contracts.
package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pactDir = "../../../../pacts"

// SummarizeRequest mirrors driver.SummarizeRequest
type SummarizeRequest struct {
	ArticleID string `json:"article_id"`
	Content   string `json:"content"`
	Stream    bool   `json:"stream"`
	Priority  string `json:"priority,omitempty"`
}

// SummarizeResponse mirrors driver.SummarizeResponse
type SummarizeResponse struct {
	Success          bool     `json:"success"`
	ArticleID        string   `json:"article_id"`
	Summary          string   `json:"summary"`
	Model            string   `json:"model"`
	PromptTokens     *int     `json:"prompt_tokens,omitempty"`
	CompletionTokens *int     `json:"completion_tokens,omitempty"`
	TotalDurationMs  *float64 `json:"total_duration_ms,omitempty"`
}

func newNewsCreatorPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "pre-processor",
		Provider: "news-creator",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func TestNewsCreatorSummarizeContract(t *testing.T) {
	mockProvider := newNewsCreatorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the LLM model is loaded and ready").
		UponReceiving("a summarize request for a normal article").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/api/v1/summarize"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"article_id": matchers.Like("article-001"),
				"content":    matchers.Like("This is a sufficiently long article content for summarization testing. It needs to be at least one hundred characters long to pass the minimum content length validation check."),
				"stream":     matchers.Like(false),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"success":    matchers.Like(true),
				"article_id": matchers.Like("article-001"),
				"summary":    matchers.Like("これはテスト記事の要約です。"),
				"model":      matchers.Like("gemma4-e4b-q4km"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			apiURL := fmt.Sprintf("http://%s:%d/api/v1/summarize", config.Host, config.Port)

			payload := SummarizeRequest{
				ArticleID: "article-001",
				Content:   "This is a sufficiently long article content for summarization testing.",
				Stream:    false,
			}
			jsonData, _ := json.Marshal(payload)

			req, err := http.NewRequestWithContext(context.Background(), "POST", apiURL, strings.NewReader(string(jsonData)))
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			var apiResp SummarizeResponse
			if err := json.Unmarshal(body, &apiResp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			assert.True(t, apiResp.Success)
			assert.NotEmpty(t, apiResp.Summary)
			assert.NotEmpty(t, apiResp.Model)
			assert.Equal(t, "article-001", apiResp.ArticleID)
			return nil
		})
	require.NoError(t, err)
}

func TestNewsCreatorSummarizeQueueFullContract(t *testing.T) {
	mockProvider := newNewsCreatorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the LLM queue is full").
		UponReceiving("a summarize request when queue is full").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/api/v1/summarize"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"article_id": matchers.Like("article-overflow"),
				"content":    matchers.Like("Content that arrives when the summarization queue is full. This article has sufficient length to pass the minimum content validation check of one hundred characters in the news-creator service."),
				"stream":     matchers.Like(false),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 429,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
				"Retry-After":  matchers.Like("30"),
			},
			Body: matchers.MapMatcher{
				"error": matchers.Like("queue full"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			apiURL := fmt.Sprintf("http://%s:%d/api/v1/summarize", config.Host, config.Port)

			payload := SummarizeRequest{
				ArticleID: "article-overflow",
				Content:   "Content that arrives when queue is full.",
			}
			jsonData, _ := json.Marshal(payload)

			req, _ := http.NewRequestWithContext(context.Background(), "POST", apiURL, strings.NewReader(string(jsonData)))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()

			assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
			assert.NotEmpty(t, resp.Header.Get("Retry-After"))
			return nil
		})
	require.NoError(t, err)
}

func TestNewsCreatorSummarizeWithPriorityContract(t *testing.T) {
	mockProvider := newNewsCreatorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the LLM model is loaded and ready").
		UponReceiving("a summarize request with HIGH priority").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/api/v1/summarize"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"article_id": matchers.Like("article-rt"),
				"content":    matchers.Like("Real-time article content requiring immediate summarization. This article covers breaking news about artificial intelligence and needs to be processed with high priority for immediate delivery."),
				"stream":     matchers.Like(false),
				"priority":   matchers.String("high"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"success":    matchers.Like(true),
				"article_id": matchers.Like("article-rt"),
				"summary":    matchers.Like("リアルタイム記事の要約。"),
				"model":      matchers.Like("gemma4-e4b-q4km"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			apiURL := fmt.Sprintf("http://%s:%d/api/v1/summarize", config.Host, config.Port)

			payload := SummarizeRequest{
				ArticleID: "article-rt",
				Content:   "Real-time article content requiring immediate summarization.",
				Priority:  "high",
			}
			jsonData, _ := json.Marshal(payload)

			req, _ := http.NewRequestWithContext(context.Background(), "POST", apiURL, strings.NewReader(string(jsonData)))
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			var apiResp SummarizeResponse
			if err := json.Unmarshal(body, &apiResp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			assert.True(t, apiResp.Success)
			assert.NotEmpty(t, apiResp.Summary)
			return nil
		})
	require.NoError(t, err)
}
