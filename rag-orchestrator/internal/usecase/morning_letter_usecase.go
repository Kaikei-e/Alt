package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"rag-orchestrator/internal/domain"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
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
	articleClient    domain.ArticleClient
	retrieveUC       RetrieveContextUsecase
	promptBuilder    MorningLetterPromptBuilder
	llmClient        domain.LLMClient
	maxTokens        int
	maxPromptTokens  int
	temporalBoostCfg TemporalBoostConfig
	logger           *slog.Logger
}

// NewMorningLetterUsecase creates a new morning letter usecase.
// If temporalBoostCfg is zero-valued, defaults are used.
// maxTokens controls the LLM generation token limit (0 defaults to 4096).
// maxPromptTokens controls the maximum prompt tokens for context limiting (0 defaults to 6000).
func NewMorningLetterUsecase(
	articleClient domain.ArticleClient,
	retrieveUC RetrieveContextUsecase,
	promptBuilder MorningLetterPromptBuilder,
	llmClient domain.LLMClient,
	maxTokens int,
	maxPromptTokens int,
	temporalBoostCfg TemporalBoostConfig,
	logger *slog.Logger,
) MorningLetterUsecase {
	// Apply defaults if config is zero-valued
	if temporalBoostCfg.Boost6h == 0 {
		temporalBoostCfg = DefaultTemporalBoostConfig()
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	if maxPromptTokens <= 0 {
		maxPromptTokens = 6000
	}
	return &morningLetterUsecase{
		articleClient:    articleClient,
		retrieveUC:       retrieveUC,
		promptBuilder:    promptBuilder,
		llmClient:        llmClient,
		maxTokens:        maxTokens,
		maxPromptTokens:  maxPromptTokens,
		temporalBoostCfg: temporalBoostCfg,
		logger:           logger,
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
		slog.String("query_preview", queryLogPreview(input.Query)),
		slog.Int("query_len", len(input.Query)),
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

	// 5.5 Dynamic token-based context limiting (same pattern as answer_with_rag_usecase)
	// Prevents prompt from exceeding LLM context window.
	// Japanese text averages ~3 characters per token.
	maxMorningLetterPromptTokens := u.maxPromptTokens
	estimatedTokens := 600 // morning letter system prompt overhead (larger than augur)
	var limitedContexts []ContextItem
	for _, ctx := range boostedContexts {
		chunkTokens := estimateTokens(ctx.ChunkText)
		if estimatedTokens+chunkTokens > maxMorningLetterPromptTokens && len(limitedContexts) > 0 {
			break
		}
		estimatedTokens += chunkTokens
		limitedContexts = append(limitedContexts, ctx)
	}
	if len(limitedContexts) < len(boostedContexts) {
		u.logger.Info("morning_letter_context_limited_by_tokens",
			slog.Int("original_count", len(boostedContexts)),
			slog.Int("limited_count", len(limitedContexts)),
			slog.Int("estimated_tokens", estimatedTokens))
	}
	boostedContexts = limitedContexts

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
	response, err := u.llmClient.Chat(ctx, messages, u.maxTokens)
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
// using configurable boost factors from TemporalBoostConfig.
func (u *morningLetterUsecase) applyTemporalBoost(contexts []ContextItem, now time.Time) []ContextItem {
	for i := range contexts {
		publishedAt, err := time.Parse(time.RFC3339, contexts[i].PublishedAt)
		if err != nil {
			continue
		}
		hoursSince := now.Sub(publishedAt).Hours()

		// Use configurable temporal boost factors
		boost := u.temporalBoostCfg.GetBoostFactor(hoursSince)
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
	jsonStart := strings.IndexByte(text, '{')
	jsonEnd := -1

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

	// The prompt (morning_letter_prompt_builder.go) instructs the LLM to
	// return "article_refs": [1, 3, 5] — 1-based positional indices into the
	// numbered context list shown in the user message ("[%d] title (...)").
	// domain.TopicSummary.ArticleRefs is []ArticleRef (a UUID-bearing struct),
	// which the LLM never emits, so we unmarshal into an intermediate DTO
	// that matches the prompt's actual contract and then hydrate the real
	// ArticleRef structs from boostedContexts.
	var raw rawMorningLetterResponse
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal topics response: %w", err)
	}

	topics := make([]domain.TopicSummary, 0, len(raw.Topics))
	for _, rt := range raw.Topics {
		topics = append(topics, domain.TopicSummary{
			Topic:       rt.Topic,
			Headline:    rt.Headline,
			Summary:     rt.Summary,
			Importance:  rt.Importance,
			Keywords:    rt.Keywords,
			ArticleRefs: enrichArticleRefs(rt.ArticleRefs, contexts),
		})
	}

	return topics, nil
}

// rawMorningLetterResponse mirrors the LLM's actual JSON contract (see
// morning_letter_prompt_builder.go), where article_refs are 1-based
// positional indices rather than domain.ArticleRef structs.
type rawMorningLetterResponse struct {
	Topics []rawTopicSummary `json:"topics"`
	Meta   domain.TopicsMeta `json:"meta"`
}

type rawTopicSummary struct {
	Topic       string   `json:"topic"`
	Headline    string   `json:"headline"`
	Summary     string   `json:"summary"`
	Importance  float32  `json:"importance"`
	ArticleRefs []int    `json:"article_refs"`
	Keywords    []string `json:"keywords"`
}

// enrichArticleRefs maps the LLM's 1-based positional article_refs indices
// back to the numbered context list the prompt showed it and hydrates them
// into domain.ArticleRef with the real article UUID/title/URL. Out-of-range
// indices, duplicates, and contexts without a parseable ArticleID are
// dropped rather than surfaced as fake refs.
func enrichArticleRefs(refs []int, contexts []ContextItem) []domain.ArticleRef {
	enriched := make([]domain.ArticleRef, 0, len(refs))
	seen := make(map[int]struct{}, len(refs))
	for _, idx := range refs {
		if _, dup := seen[idx]; dup {
			continue
		}
		seen[idx] = struct{}{}

		pos := idx - 1
		if pos < 0 || pos >= len(contexts) {
			continue
		}
		ctx := contexts[pos]

		articleID, err := uuid.Parse(ctx.ArticleID)
		if err != nil {
			continue
		}

		publishedAt, _ := time.Parse(time.RFC3339, ctx.PublishedAt)
		enriched = append(enriched, domain.ArticleRef{
			ID:          articleID,
			Title:       ctx.Title,
			URL:         ctx.URL,
			PublishedAt: publishedAt,
		})
	}
	return enriched
}
