// Package contract contains Consumer-Driven Contract tests for alt-backend → pre-processor.
//
// These tests verify that alt-backend's expectations of the pre-processor
// Connect-RPC API are documented as Pact contracts. The generated pact files
// are later verified against the real pre-processor implementation.
package contract

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ppv2 "alt/gen/proto/services/preprocessor/v2"
	"alt/gen/proto/services/preprocessor/v2/preprocessorv2connect"
)

const pactDir = "../../../../pacts"

func newPreProcessorPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "pre-processor",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func newConnectClient(config consumer.MockServerConfig) preprocessorv2connect.PreProcessorServiceClient {
	return preprocessorv2connect.NewPreProcessorServiceClient(
		http.DefaultClient,
		fmt.Sprintf("http://%s:%d", config.Host, config.Port),
		connect.WithProtoJSON(),
	)
}

func TestPreProcessorSummarizeContract(t *testing.T) {
	mockProvider := newPreProcessorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("an article exists with id article-123").
		UponReceiving("a Summarize request for article-123").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.preprocessor.v2.PreProcessorService/Summarize"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"articleId": matchers.String("article-123"),
				"title":     matchers.Like("Test Article"),
				"content":   matchers.Like("This is a test article content for summarization."),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"success":   matchers.Like(true),
				"summary":   matchers.Like("This is a generated summary."),
				"articleId": matchers.String("article-123"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newConnectClient(config)
			resp, err := client.Summarize(context.Background(), connect.NewRequest(&ppv2.SummarizeRequest{
				ArticleId: "article-123",
				Title:     "Test Article",
				Content:   "This is a test article content for summarization.",
			}))
			if err != nil {
				return fmt.Errorf("Summarize failed: %w", err)
			}

			assert.True(t, resp.Msg.Success)
			assert.NotEmpty(t, resp.Msg.Summary)
			assert.Equal(t, "article-123", resp.Msg.ArticleId)
			return nil
		})
	require.NoError(t, err)
}

func TestPreProcessorQueueSummarizeContract(t *testing.T) {
	mockProvider := newPreProcessorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the summarization queue is available").
		UponReceiving("a QueueSummarize request for article-456").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.preprocessor.v2.PreProcessorService/QueueSummarize"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"articleId": matchers.Like("article-456"),
				"title":     matchers.Like("Async Test Article"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"jobId":   matchers.Like("job-001"),
				"status":  matchers.Like("pending"),
				"message": matchers.Like("Job queued successfully"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newConnectClient(config)
			resp, err := client.QueueSummarize(context.Background(), connect.NewRequest(&ppv2.QueueSummarizeRequest{
				ArticleId: "article-456",
				Title:     "Async Test Article",
			}))
			if err != nil {
				return fmt.Errorf("QueueSummarize failed: %w", err)
			}

			assert.NotEmpty(t, resp.Msg.JobId)
			assert.Equal(t, "pending", resp.Msg.Status)
			return nil
		})
	require.NoError(t, err)
}

func TestPreProcessorGetSummarizeStatusContract(t *testing.T) {
	mockProvider := newPreProcessorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a completed summarize job exists with id job-789").
		UponReceiving("a GetSummarizeStatus request for job-789").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.preprocessor.v2.PreProcessorService/GetSummarizeStatus"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"jobId": matchers.String("job-789"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"jobId":     matchers.String("job-789"),
				"status":    matchers.Like("completed"),
				"summary":   matchers.Like("Generated summary text"),
				"articleId": matchers.Like("article-456"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newConnectClient(config)
			resp, err := client.GetSummarizeStatus(context.Background(), connect.NewRequest(&ppv2.GetSummarizeStatusRequest{
				JobId: "job-789",
			}))
			if err != nil {
				return fmt.Errorf("GetSummarizeStatus failed: %w", err)
			}

			assert.Equal(t, "job-789", resp.Msg.JobId)
			assert.Equal(t, "completed", resp.Msg.Status)
			assert.NotEmpty(t, resp.Msg.Summary)
			return nil
		})
	require.NoError(t, err)
}
