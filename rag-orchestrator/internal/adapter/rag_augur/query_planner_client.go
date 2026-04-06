package rag_augur

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"rag-orchestrator/internal/domain"
)

// planQueryRequest is the request payload for the plan-query endpoint.
type planQueryRequest struct {
	Query               string                `json:"query"`
	ConversationHistory []ConversationMessage `json:"conversation_history,omitempty"`
	ArticleID           *string               `json:"article_id,omitempty"`
	ArticleTitle        *string               `json:"article_title,omitempty"`
	LastAnswerScope     *string               `json:"last_answer_scope,omitempty"`
	Priority            string                `json:"priority"`
}

// queryPlanJSON mirrors the news-creator QueryPlan schema.
type queryPlanJSON struct {
	ResolvedQuery    string   `json:"resolved_query"`
	SearchQueries    []string `json:"search_queries"`
	Intent           string   `json:"intent"`
	RetrievalPolicy  string   `json:"retrieval_policy"`
	AnswerFormat     string   `json:"answer_format"`
	ShouldClarify    bool     `json:"should_clarify"`
	ClarificationMsg *string  `json:"clarification_msg,omitempty"`
	TopicEntities    []string `json:"topic_entities"`
}

// planQueryResponse mirrors the news-creator PlanQueryResponse.
type planQueryResponse struct {
	Plan             queryPlanJSON `json:"plan"`
	OriginalQuery    string        `json:"original_query"`
	Model            string        `json:"model"`
	ProcessingTimeMs *float64      `json:"processing_time_ms,omitempty"`
}

// QueryPlannerClient calls the news-creator /api/v1/plan-query endpoint.
type QueryPlannerClient struct {
	BaseURL string
	Client  *http.Client
	logger  *slog.Logger
}

// NewQueryPlannerClient constructs a new QueryPlannerClient.
func NewQueryPlannerClient(baseURL string, timeoutSec int, logger *slog.Logger) *QueryPlannerClient {
	return &QueryPlannerClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		logger:  logger,
	}
}

// PlanQuery calls the news-creator plan-query endpoint and returns a QueryPlan.
func (c *QueryPlannerClient) PlanQuery(ctx context.Context, input domain.QueryPlannerInput) (*domain.QueryPlan, error) {
	reqBody := planQueryRequest{
		Query:    input.Query,
		Priority: "high",
	}

	for _, msg := range input.ConversationHistory {
		reqBody.ConversationHistory = append(reqBody.ConversationHistory, ConversationMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	if input.ArticleID != "" {
		reqBody.ArticleID = &input.ArticleID
	}
	if input.ArticleTitle != "" {
		reqBody.ArticleTitle = &input.ArticleTitle
	}
	if input.LastAnswerScope != "" {
		reqBody.LastAnswerScope = &input.LastAnswerScope
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal plan-query request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/plan-query", c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create plan-query request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plan-query request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plan-query returned status: %d", resp.StatusCode)
	}

	var pqResp planQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&pqResp); err != nil {
		return nil, fmt.Errorf("decode plan-query response: %w", err)
	}

	plan := &domain.QueryPlan{
		ResolvedQuery:   pqResp.Plan.ResolvedQuery,
		SearchQueries:   pqResp.Plan.SearchQueries,
		Intent:          pqResp.Plan.Intent,
		RetrievalPolicy: normalizeRetrievalPolicy(pqResp.Plan.RetrievalPolicy),
		AnswerFormat:    pqResp.Plan.AnswerFormat,
		ShouldClarify:   pqResp.Plan.ShouldClarify,
		TopicEntities:   pqResp.Plan.TopicEntities,
	}
	if pqResp.Plan.ClarificationMsg != nil {
		plan.ClarificationMsg = *pqResp.Plan.ClarificationMsg
	}

	// Sanity checks: prevent broken plans from reaching the pipeline
	if len([]rune(plan.ResolvedQuery)) < 2 {
		plan.ResolvedQuery = input.Query // fall back to original
	}
	if plan.ShouldClarify && len(input.ConversationHistory) == 0 {
		// Single-turn queries should never need clarification
		plan.ShouldClarify = false
	}
	if len(plan.SearchQueries) == 0 {
		plan.SearchQueries = []string{plan.ResolvedQuery}
	}

	c.logger.Info("plan_query_completed",
		slog.String("resolved_query", plan.ResolvedQuery),
		slog.String("intent", plan.Intent),
		slog.String("retrieval_policy", plan.RetrievalPolicy),
		slog.Bool("should_clarify", plan.ShouldClarify),
		slog.Int("search_queries", len(plan.SearchQueries)))

	return plan, nil
}

// normalizeRetrievalPolicy maps LLM-returned policy strings to valid domain constants.
func normalizeRetrievalPolicy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "global_only", "global", "general", "search_queries":
		return "global_only"
	case "article_only", "article":
		return "article_only"
	case "tool_only", "tool":
		return "tool_only"
	case "no_retrieval", "none":
		return "no_retrieval"
	default:
		return "global_only"
	}
}
