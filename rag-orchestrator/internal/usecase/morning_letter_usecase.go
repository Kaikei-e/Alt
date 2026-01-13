package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"rag-orchestrator/internal/domain"
	"sort"
	"time"
)

// MorningLetterInput defines the input for morning letter extraction
type MorningLetterInput struct {
	Query       string // User query (e.g., "important news from yesterday")
	WithinHours int    // Time window (default: 24)
	TopicLimit  int    // Max topics to return (default: 5)
	Locale      string // Response language
}

// MorningLetterOutput defines the output for morning letter
type MorningLetterOutput struct {
	Topics          []domain.TopicSummary `json:"topics"`
	TimeWindow      TimeWindow            `json:"time_window"`
	ArticlesScanned int                   `json:"articles_scanned"`
	GenerationInfo  GenerationInfo        `json:"generation_info"`
}

// TimeWindow represents the time range for the query
type TimeWindow struct {
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
}

// GenerationInfo contains metadata about the LLM generation
type GenerationInfo struct {
	Model    string `json:"model"`
	Fallback bool   `json:"fallback"`
}

// MorningLetterUsecase defines the interface for morning letter extraction
type MorningLetterUsecase interface {
	Execute(ctx context.Context, input MorningLetterInput) (*MorningLetterOutput, error)
}

type morningLetterUsecase struct {
	articleClient domain.ArticleClient
	retrieveUC    RetrieveContextUsecase
	promptBuilder MorningLetterPromptBuilder
	llmClient     domain.LLMClient
	logger        *slog.Logger
}

// NewMorningLetterUsecase creates a new morning letter usecase
func NewMorningLetterUsecase(
	articleClient domain.ArticleClient,
	retrieveUC RetrieveContextUsecase,
	promptBuilder MorningLetterPromptBuilder,
	llmClient domain.LLMClient,
	logger *slog.Logger,
) MorningLetterUsecase {
	return &morningLetterUsecase{
		articleClient: articleClient,
		retrieveUC:    retrieveUC,
		promptBuilder: promptBuilder,
		llmClient:     llmClient,
		logger:        logger,
	}
}

// Execute extracts important topics from recent articles
func (u *morningLetterUsecase) Execute(ctx context.Context, input MorningLetterInput) (*MorningLetterOutput, error) {
	// 1. Validate and set defaults
	withinHours := input.WithinHours
	if withinHours <= 0 {
		withinHours = 24
	}
	if withinHours > 168 { // Max 7 days
		withinHours = 168
	}

	topicLimit := input.TopicLimit
	if topicLimit <= 0 {
		topicLimit = 10
	}
	if topicLimit > 20 {
		topicLimit = 20
	}

	locale := input.Locale
	if locale == "" {
		locale = "ja"
	}

	now := time.Now()
	since := now.Add(-time.Duration(withinHours) * time.Hour)

	u.logger.Info("morning_letter_started",
		slog.String("query", input.Query),
		slog.Int("within_hours", withinHours),
		slog.Int("topic_limit", topicLimit),
		slog.String("locale", locale))

	// 2. Fetch recent articles from alt-backend (limit=0 means no limit, relying on time constraint only)
	articles, err := u.articleClient.GetRecentArticles(ctx, withinHours, 0)
	if err != nil {
		u.logger.Error("failed to fetch recent articles", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to fetch recent articles: %w", err)
	}

	u.logger.Info("fetched recent articles", slog.Int("count", len(articles)))

	if len(articles) == 0 {
		return &MorningLetterOutput{
			Topics:          []domain.TopicSummary{},
			TimeWindow:      TimeWindow{Since: since, Until: now},
			ArticlesScanned: 0,
			GenerationInfo:  GenerationInfo{Model: "none", Fallback: true},
		}, nil
	}

	// 3. Extract article IDs for context retrieval
	articleIDs := make([]string, len(articles))
	for i, a := range articles {
		articleIDs[i] = a.ID.String()
	}

	// 4. Retrieve context with temporal filtering
	retrieveOutput, err := u.retrieveUC.Execute(ctx, RetrieveContextInput{
		Query:               input.Query,
		CandidateArticleIDs: articleIDs,
	})
	if err != nil {
		u.logger.Error("failed to retrieve context", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to retrieve context: %w", err)
	}

	u.logger.Info("retrieved context", slog.Int("context_count", len(retrieveOutput.Contexts)))

	if len(retrieveOutput.Contexts) == 0 {
		return &MorningLetterOutput{
			Topics:          []domain.TopicSummary{},
			TimeWindow:      TimeWindow{Since: since, Until: now},
			ArticlesScanned: len(articles),
			GenerationInfo:  GenerationInfo{Model: "none", Fallback: true},
		}, nil
	}

	// 5. Apply temporal boost to context scores
	boostedContexts := u.applyTemporalBoost(retrieveOutput.Contexts, now)

	// 6. Build morning letter prompt
	messages, err := u.promptBuilder.Build(MorningLetterPromptInput{
		Query:      input.Query,
		Contexts:   boostedContexts,
		Since:      since,
		Until:      now,
		TopicLimit: topicLimit,
		Locale:     locale,
	})
	if err != nil {
		u.logger.Error("failed to build prompt", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// 7. Generate topics via LLM
	response, err := u.llmClient.Chat(ctx, messages, 1500)
	if err != nil {
		u.logger.Error("LLM generation failed", slog.String("error", err.Error()))
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// 8. Parse and validate response
	topics, err := u.parseTopicsResponse(response.Text, boostedContexts)
	if err != nil {
		u.logger.Warn("failed to parse topics response, returning empty",
			slog.String("error", err.Error()),
			slog.String("raw_response", truncate(response.Text, 500)))
		return &MorningLetterOutput{
			Topics:          []domain.TopicSummary{},
			TimeWindow:      TimeWindow{Since: since, Until: now},
			ArticlesScanned: len(articles),
			GenerationInfo:  GenerationInfo{Model: "llm", Fallback: true},
		}, nil
	}

	u.logger.Info("morning_letter_completed",
		slog.Int("topics_extracted", len(topics)),
		slog.Int("articles_scanned", len(articles)))

	return &MorningLetterOutput{
		Topics:          topics,
		TimeWindow:      TimeWindow{Since: since, Until: now},
		ArticlesScanned: len(articles),
		GenerationInfo: GenerationInfo{
			Model:    "llm",
			Fallback: false,
		},
	}, nil
}

// applyTemporalBoost increases scores for more recent articles
func (u *morningLetterUsecase) applyTemporalBoost(contexts []ContextItem, now time.Time) []ContextItem {
	for i := range contexts {
		publishedAt, err := time.Parse(time.RFC3339, contexts[i].PublishedAt)
		if err != nil {
			continue
		}
		hoursSince := now.Sub(publishedAt).Hours()

		// Temporal boost factor
		var boost float32 = 1.0
		switch {
		case hoursSince <= 6:
			boost = 1.3 // 30% boost for last 6 hours
		case hoursSince <= 12:
			boost = 1.15 // 15% boost for 6-12 hours
		case hoursSince <= 18:
			boost = 1.05 // 5% boost for 12-18 hours
		}
		contexts[i].Score *= boost
	}

	// Re-sort by boosted score
	sort.Slice(contexts, func(i, j int) bool {
		return contexts[i].Score > contexts[j].Score
	})

	return contexts
}

// parseTopicsResponse parses the LLM JSON response into TopicSummary slice
func (u *morningLetterUsecase) parseTopicsResponse(text string, contexts []ContextItem) ([]domain.TopicSummary, error) {
	// Try to extract JSON from the response
	jsonStart := -1
	jsonEnd := -1

	for i := 0; i < len(text); i++ {
		if text[i] == '{' && jsonStart == -1 {
			jsonStart = i
		}
	}

	if jsonStart == -1 {
		return nil, fmt.Errorf("no JSON object found in response")
	}

	// Find matching closing brace
	depth := 0
	for i := jsonStart; i < len(text); i++ {
		if text[i] == '{' {
			depth++
		} else if text[i] == '}' {
			depth--
			if depth == 0 {
				jsonEnd = i + 1
				break
			}
		}
	}

	if jsonEnd == -1 {
		return nil, fmt.Errorf("incomplete JSON object")
	}

	jsonStr := text[jsonStart:jsonEnd]

	var response domain.MorningLetterResponse
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal topics response: %w", err)
	}

	// Enrich article refs with actual article data from contexts
	for i := range response.Topics {
		enrichedRefs := make([]domain.ArticleRef, 0)
		for _, refIdx := range response.Topics[i].ArticleRefs {
			// refIdx is 1-based index
			if refIdx.ID.String() != "" {
				enrichedRefs = append(enrichedRefs, refIdx)
			}
		}
		response.Topics[i].ArticleRefs = enrichedRefs
	}

	return response.Topics, nil
}
