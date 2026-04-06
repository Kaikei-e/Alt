//go:build contract

package contract

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"rag-orchestrator/internal/adapter/rag_augur"
	"rag-orchestrator/internal/domain"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlanQueryContract verifies the /api/v1/plan-query endpoint contract.
// Consumer: rag-orchestrator, Provider: news-creator.
func TestPlanQueryContract(t *testing.T) {
	mockProvider := newNewsCreatorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the LLM model is loaded and ready for query planning").
		UponReceiving("a plan-query request from rag-orchestrator").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/api/v1/plan-query"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"query":    matchers.Like("イランの石油危機はなぜ起きた？"),
				"priority": matchers.Like("high"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"plan": matchers.Like(map[string]interface{}{
					"resolved_query":   "イランの石油危機が発生した背景と直接的原因",
					"search_queries":   matchers.EachLike("Iran oil crisis causes", 1),
					"intent":           "causal_explanation",
					"retrieval_policy": "global_only",
					"answer_format":    "causal_analysis",
					"should_clarify":   false,
					"topic_entities":   matchers.EachLike("イラン", 1),
				}),
				"original_query": matchers.Like("イランの石油危機はなぜ起きた？"),
				"model":          matchers.Like("gemma4-e4b-12k"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
			client := rag_augur.NewQueryPlannerClient(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				5,
				logger,
			)

			plan, err := client.PlanQuery(context.Background(), domain.QueryPlannerInput{
				Query: "イランの石油危機はなぜ起きた？",
			})
			if err != nil {
				return fmt.Errorf("PlanQuery failed: %w", err)
			}

			assert.Equal(t, "causal_explanation", plan.Intent)
			assert.Equal(t, "global_only", plan.RetrievalPolicy)
			assert.False(t, plan.ShouldClarify)
			assert.NotEmpty(t, plan.ResolvedQuery)
			assert.NotEmpty(t, plan.SearchQueries)
			return nil
		})
	require.NoError(t, err)
}

// TestPlanQueryWithHistoryContract verifies conversation history is passed correctly.
func TestPlanQueryWithHistoryContract(t *testing.T) {
	mockProvider := newNewsCreatorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the LLM model is loaded and ready for query planning").
		UponReceiving("a plan-query request with conversation history").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/api/v1/plan-query"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"query": matchers.Like("それについて詳しく"),
				"conversation_history": matchers.EachLike(map[string]interface{}{
					"role":    "user",
					"content": "previous question",
				}, 1),
				"priority": matchers.Like("high"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"plan": matchers.Like(map[string]interface{}{
					"resolved_query":   "resolved standalone query",
					"search_queries":   matchers.EachLike("search query", 1),
					"intent":           "general",
					"retrieval_policy": "global_only",
					"answer_format":    "summary",
					"should_clarify":   false,
					"topic_entities":   matchers.EachLike("entity", 1),
				}),
				"original_query": matchers.Like("それについて詳しく"),
				"model":          matchers.Like("gemma4-e4b-12k"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
			client := rag_augur.NewQueryPlannerClient(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				5,
				logger,
			)

			plan, err := client.PlanQuery(context.Background(), domain.QueryPlannerInput{
				Query: "それについて詳しく",
				ConversationHistory: []domain.Message{
					{Role: "user", Content: "previous question"},
					{Role: "assistant", Content: "previous answer"},
				},
			})
			if err != nil {
				return fmt.Errorf("PlanQuery with history failed: %w", err)
			}

			assert.NotEmpty(t, plan.ResolvedQuery)
			assert.False(t, plan.ShouldClarify)
			return nil
		})
	require.NoError(t, err)
}
