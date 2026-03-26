package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

type answerWithRAGUsecase struct {
	retrieve          RetrieveContextUsecase
	promptBuilder     PromptBuilder
	llmClient         domain.LLMClient
	validator         OutputValidator
	qualityAssessor   *RetrievalQualityAssessor
	queryExpander     domain.QueryExpander
	queryClassifier   *QueryClassifier
	toolDispatcher    *ToolDispatcher
	maxChunks         int
	maxTokens         int
	maxPromptTokens   int
	promptVersion     string
	defaultLocale     string
	heartbeatInterval time.Duration
	cache             *expirable.LRU[string, *AnswerWithRAGOutput]
	logger            *slog.Logger
	strategies        map[IntentType]RetrievalStrategy
	generalStrategy   RetrievalStrategy
}

// NewAnswerWithRAGUsecase wires together the components needed to generate a RAG answer.
func NewAnswerWithRAGUsecase(
	retrieve RetrieveContextUsecase,
	promptBuilder PromptBuilder,
	llmClient domain.LLMClient,
	validator OutputValidator,
	maxChunks, maxTokens, maxPromptTokens int,
	promptVersion, defaultLocale string,
	logger *slog.Logger,
	opts ...AnswerUsecaseOption,
) AnswerWithRAGUsecase {
	if maxPromptTokens <= 0 {
		maxPromptTokens = 6000
	}
	cfg := answerUsecaseConfig{
		cacheSize:         256,
		cacheTTL:          10 * time.Minute,
		heartbeatInterval: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	generalStrat := NewGeneralStrategy(retrieve)
	strategies := cfg.strategies
	if strategies == nil {
		strategies = make(map[IntentType]RetrievalStrategy)
	}
	return &answerWithRAGUsecase{
		retrieve:          retrieve,
		promptBuilder:     promptBuilder,
		llmClient:         llmClient,
		validator:         validator,
		qualityAssessor:   cfg.qualityAssessor,
		queryExpander:     cfg.queryExpander,
		queryClassifier:   cfg.queryClassifier,
		toolDispatcher:    cfg.toolDispatcher,
		maxChunks:         maxChunks,
		maxTokens:         maxTokens,
		maxPromptTokens:   maxPromptTokens,
		promptVersion:     promptVersion,
		defaultLocale:     defaultLocale,
		heartbeatInterval: cfg.heartbeatInterval,
		cache:             expirable.NewLRU[string, *AnswerWithRAGOutput](cfg.cacheSize, nil, cfg.cacheTTL),
		logger:            logger,
		strategies:        strategies,
		generalStrategy:   generalStrat,
	}
}

// AnswerUsecaseOption configures the answer usecase.
type AnswerUsecaseOption func(cfg *answerUsecaseConfig)

type answerUsecaseConfig struct {
	cacheSize         int
	cacheTTL          time.Duration
	heartbeatInterval time.Duration
	strategies        map[IntentType]RetrievalStrategy
	qualityAssessor   *RetrievalQualityAssessor
	queryExpander     domain.QueryExpander
	queryClassifier   *QueryClassifier
	toolDispatcher    *ToolDispatcher
}

// WithCacheConfig sets the cache size and TTL.
func WithCacheConfig(size int, ttl time.Duration) AnswerUsecaseOption {
	return func(cfg *answerUsecaseConfig) {
		cfg.cacheSize = size
		cfg.cacheTTL = ttl
	}
}

// WithHeartbeatInterval sets the interval for heartbeat events during long operations.
// Default is 5 seconds. Set to 0 to disable heartbeats.
func WithHeartbeatInterval(d time.Duration) AnswerUsecaseOption {
	return func(cfg *answerUsecaseConfig) {
		cfg.heartbeatInterval = d
	}
}

// WithStrategy registers a retrieval strategy for a given intent type.
func WithStrategy(intentType IntentType, strategy RetrievalStrategy) AnswerUsecaseOption {
	return func(cfg *answerUsecaseConfig) {
		if cfg.strategies == nil {
			cfg.strategies = make(map[IntentType]RetrievalStrategy)
		}
		cfg.strategies[intentType] = strategy
	}
}

// WithQualityAssessor enables retrieval quality gating with adaptive retry.
func WithQualityAssessor(assessor *RetrievalQualityAssessor, expander domain.QueryExpander) AnswerUsecaseOption {
	return func(cfg *answerUsecaseConfig) {
		cfg.qualityAssessor = assessor
		cfg.queryExpander = expander
	}
}

// WithQueryClassifier enables smart query classification for intent routing.
func WithQueryClassifier(classifier *QueryClassifier) AnswerUsecaseOption {
	return func(cfg *answerUsecaseConfig) {
		cfg.queryClassifier = classifier
	}
}

// WithToolDispatcher enables intent-driven tool dispatch alongside retrieval.
func WithToolDispatcher(dispatcher *ToolDispatcher) AnswerUsecaseOption {
	return func(cfg *answerUsecaseConfig) {
		cfg.toolDispatcher = dispatcher
	}
}

// Execute performs the Single-Phase RAG generation with caching.
func (u *answerWithRAGUsecase) Execute(ctx context.Context, input AnswerWithRAGInput) (*AnswerWithRAGOutput, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}

	executionStart := time.Now()
	requestID := uuid.NewString()

	u.logger.Info("answer_request_started",
		slog.String("request_id", requestID),
		slog.String("query", input.Query),
		slog.Int("max_chunks", input.MaxChunks),
		slog.String("locale", input.Locale))

	// 1. Check Cache
	cacheKey := u.generateCacheKey(input)
	if val, ok := u.cache.Get(cacheKey); ok {
		u.logger.Info("cache_hit",
			slog.String("request_id", requestID),
			slog.String("cache_key", cacheKey),
			slog.String("query", input.Query))
		return val, nil
	}

	// 2. Prepare Context (Retrieval)
	// We need to retrieve contexts first to know what to prompt with.
	promptData, err := u.buildPrompt(ctx, input) // Note: buildPrompt calls retrieval
	if err != nil {
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.String("reason", err.Error()),
			slog.Int("contexts_available", len(promptData.contexts)))
		return u.prepareFallback(
			promptData.contexts,
			promptData.retrievalSetID,
			err.Error(),
			FallbackRetrievalEmpty,
			promptData.strategyUsed,
			promptData.expandedQueries,
		)
	}

	u.logger.Info("context_retrieved",
		slog.String("request_id", requestID),
		slog.Int("contexts_count", len(promptData.contexts)),
		slog.String("retrieval_set_id", promptData.retrievalSetID))

	// Log context titles for debugging
	for i, ctx := range promptData.contexts {
		u.logger.Debug("context_chunk_detail",
			slog.String("request_id", requestID),
			slog.Int("index", i+1),
			slog.String("title", ctx.Title),
			slog.Float64("score", float64(ctx.Score)),
			slog.Int("chunk_length", len(ctx.ChunkText)))
	}

	// 3. Single Stage Generation — use messages from buildPrompt directly
	messages := promptData.messages

	// Calculate approximate prompt size
	promptSize := 0
	for _, msg := range messages {
		promptSize += len(msg.Content)
	}

	firstTitle := ""
	if len(promptData.contexts) > 0 {
		firstTitle = promptData.contexts[0].Title
	}

	u.logger.Info("prompt_built",
		slog.String("request_id", requestID),
		slog.Int("chunks_used", len(promptData.contexts)),
		slog.Int("prompt_size_chars", promptSize),
		slog.Int("max_tokens", promptData.maxTokens),
		slog.String("first_context_title", firstTitle),
		slog.String("query", input.Query))

	generationStart := time.Now()
	u.logger.Info("llm_generation_started",
		slog.String("request_id", requestID),
		slog.String("retrieval_set_id", promptData.retrievalSetID))

	resp, err := u.llmClient.Chat(ctx, messages, promptData.maxTokens)
	if err != nil {
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.String("reason", fmt.Sprintf("generation failed: %v", err)),
			slog.Int("contexts_available", len(promptData.contexts)))
		return u.prepareFallback(
			promptData.contexts,
			promptData.retrievalSetID,
			fmt.Sprintf("generation failed: %v", err),
			FallbackGenerationFailed,
			promptData.strategyUsed,
			promptData.expandedQueries,
		)
	}

	generationDuration := time.Since(generationStart)
	u.logger.Info("llm_generation_completed",
		slog.String("request_id", requestID),
		slog.Int("response_length", len(resp.Text)),
		slog.Int64("generation_ms", generationDuration.Milliseconds()))

	// Validate/Parse Answer
	parsedAnswer, err := u.validator.Validate(resp.Text, promptData.contexts)
	if err != nil {
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.String("reason", fmt.Sprintf("validation failed: %v", err)),
			slog.Int("contexts_available", len(promptData.contexts)))
		return u.prepareFallback(
			promptData.contexts,
			promptData.retrievalSetID,
			fmt.Sprintf("validation failed: %v", err),
			FallbackValidationFailed,
			promptData.strategyUsed,
			promptData.expandedQueries,
		)
	}

	u.logger.Info("validation_completed",
		slog.String("request_id", requestID),
		slog.Bool("is_fallback", parsedAnswer.Fallback),
		slog.Int("citations_count", len(parsedAnswer.Citations)))

	if parsedAnswer.ShortAnswer {
		u.logger.Warn("short_answer_detected",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.Int("answer_rune_length", utf8.RuneCountInString(parsedAnswer.Answer)),
			slog.String("query", input.Query))
	}

	if parsedAnswer.Fallback {
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", promptData.retrievalSetID),
			slog.String("reason", parsedAnswer.Reason),
			slog.Int("contexts_available", len(promptData.contexts)),
			slog.String("llm_raw_response", truncate(resp.Text, 500)))
		return u.prepareFallback(
			promptData.contexts,
			promptData.retrievalSetID,
			parsedAnswer.Reason,
			FallbackLLMFallback,
			promptData.strategyUsed,
			promptData.expandedQueries,
		)
	}

	// Build Citations (Hydration)
	finalCitations := u.buildCitations(promptData.contexts, parsedAnswer.Citations)

	// Phase 4: Answer quality assessment
	qualityFlags := AssessAnswerQuality(
		parsedAnswer.Answer, input.Query, parsedAnswer.Citations, promptData.intentType,
	)
	if len(qualityFlags) > 0 {
		u.logger.Info("answer_quality_flags",
			slog.String("request_id", requestID),
			slog.Any("flags", qualityFlags))
	}

	output := &AnswerWithRAGOutput{
		Answer:    strings.TrimSpace(parsedAnswer.Answer),
		Citations: finalCitations,
		Contexts:  promptData.contexts,
		Fallback:  false,
		Reason:    "",
		Debug: AnswerDebug{
			RetrievalSetID:   promptData.retrievalSetID,
			PromptVersion:    u.promptVersion,
			ExpandedQueries:  promptData.expandedQueries,
			StrategyUsed:     promptData.strategyUsed,
			IntentType:       string(promptData.intentType),
			RetrievalQuality: string(promptData.retrievalQuality),
			RetryCount:       promptData.retryCount,
			ToolsUsed:        promptData.toolsUsed,
			QualityFlags:     qualityFlags,
		},
	}

	// 5. Store in Cache
	u.cache.Add(cacheKey, output)

	executionDuration := time.Since(executionStart)
	u.logger.Info("answer_request_completed",
		slog.String("request_id", requestID),
		slog.Int("answer_length", len(output.Answer)),
		slog.Int("citations", len(output.Citations)),
		slog.String("strategy_used", promptData.strategyUsed),
		slog.Int64("total_duration_ms", executionDuration.Milliseconds()))

	return output, nil
}

func (u *answerWithRAGUsecase) prepareFallback(
	contexts []ContextItem,
	reqID, reason string,
	category FallbackCategory,
	strategyUsed string,
	expandedQueries []string,
) (*AnswerWithRAGOutput, error) {
	return &AnswerWithRAGOutput{
		Answer:           "",
		Citations:        nil,
		Contexts:         contexts,
		Fallback:         true,
		Reason:           reason,
		FallbackCategory: category,
		Debug: AnswerDebug{
			RetrievalSetID:  reqID,
			PromptVersion:   u.promptVersion,
			ExpandedQueries: expandedQueries,
			StrategyUsed:    strategyUsed,
		},
	}, nil
}

func (u *answerWithRAGUsecase) buildCitations(contexts []ContextItem, raw []LLMCitation) []Citation {
	ctxMap := make(map[string]ContextItem, len(contexts))
	for _, ctx := range contexts {
		ctxMap[ctx.ChunkID.String()] = ctx
	}

	var citations []Citation
	for _, cite := range raw {
		var meta ContextItem
		var ok bool

		// 1. Try direct lookup (UUID)
		meta, ok = ctxMap[cite.ChunkID]
		if !ok {
			// 2. Try index lookup (e.g., "1", "2")
			// Used when prompt asks for [index] citations to save tokens
			var idx int
			if _, err := fmt.Sscanf(cite.ChunkID, "%d", &idx); err == nil {
				// 1-based index -> 0-based slice
				sliceIdx := idx - 1
				if sliceIdx >= 0 && sliceIdx < len(contexts) {
					meta = contexts[sliceIdx]
					ok = true
				}
			}
		}

		if !ok {
			continue
		}
		citations = append(citations, Citation{
			ChunkID:         meta.ChunkID.String(), // Always return real UUID to caller
			ChunkText:       meta.ChunkText,
			URL:             meta.URL,
			Title:           meta.Title,
			Score:           meta.Score, // Use retrieval score
			DocumentVersion: meta.DocumentVersion,
		})
	}

	return citations
}

// ArticleContext carries article-scoped metadata into prompt building.
type ArticleContext struct {
	ArticleID string
	Title     string
	Truncated bool // true if token limits caused chunk truncation
}

type promptBuildResult struct {
	retrievalSetID   string
	contexts         []ContextItem
	messages         []domain.Message
	maxTokens        int
	expandedQueries  []string
	strategyUsed     string
	intentType       IntentType
	toolsUsed        []string
	articleContext   *ArticleContext
	retrievalQuality QualityVerdict
	retryCount       int
}

func (u *answerWithRAGUsecase) buildPrompt(ctx context.Context, input AnswerWithRAGInput) (*promptBuildResult, error) {
	maxChunks := input.MaxChunks
	if maxChunks <= 0 {
		maxChunks = u.maxChunks
	}
	maxTokens := input.MaxTokens
	if maxTokens <= 0 {
		maxTokens = u.maxTokens
	}

	result := &promptBuildResult{
		retrievalSetID: uuid.NewString(),
		maxTokens:      maxTokens,
	}

	// Parse intent from raw query
	intent := ResolveQueryIntent(input.Query, input.ConversationHistory)

	// Smart classification: if not article-scoped, use classifier for richer intent
	if intent.IntentType != IntentArticleScoped && u.queryClassifier != nil {
		classified := u.queryClassifier.Classify(ctx, intent.UserQuestion)
		if classified != IntentGeneral {
			intent.IntentType = classified
		}
	}

	strategy := u.selectStrategy(intent.IntentType)
	result.strategyUsed = strategy.Name()
	result.intentType = intent.IntentType

	u.logger.Info("query_intent_parsed",
		slog.String("intent_type", string(intent.IntentType)),
		slog.String("article_id", intent.ArticleID),
		slog.String("strategy", strategy.Name()))

	// Use intent.UserQuestion for retrieval (metadata stripped)
	retrieveInput := RetrieveContextInput{
		Query:               intent.UserQuestion,
		ConversationHistory: input.ConversationHistory,
	}
	if len(input.CandidateArticleIDs) > 0 {
		retrieveInput.CandidateArticleIDs = input.CandidateArticleIDs
	}

	retrieved, err := strategy.Retrieve(ctx, retrieveInput, intent)

	// Agentic RAG: for article-scoped follow-ups, augment with general re-retrieval.
	// The article's own chunks provide scoped context, while the global index
	// surfaces related articles that may answer the follow-up question better.
	if intent.IntentType == IntentArticleScoped && len(input.ConversationHistory) > 0 && err == nil && retrieved != nil {
		u.logger.Info("agentic_reretrieval_started",
			slog.String("article_id", intent.ArticleID),
			slog.String("query", intent.UserQuestion))

		generalInput := RetrieveContextInput{
			Query:               intent.UserQuestion,
			ConversationHistory: input.ConversationHistory,
		}
		generalResult, genErr := u.generalStrategy.Retrieve(ctx, generalInput, intent)
		if genErr != nil {
			u.logger.Warn("agentic_reretrieval_failed",
				slog.String("article_id", intent.ArticleID),
				slog.String("error", genErr.Error()))
		}
		if genErr == nil && generalResult != nil && len(generalResult.Contexts) > 0 {
			before := len(retrieved.Contexts)
			retrieved = mergeContexts(retrieved, generalResult)
			u.logger.Info("agentic_reretrieval_completed",
				slog.Int("article_chunks", before),
				slog.Int("general_chunks", len(generalResult.Contexts)),
				slog.Int("merged_total", len(retrieved.Contexts)))
			result.strategyUsed = "article_scoped+general"
		}
	}

	// 2-stage fallback for article_scoped
	if intent.IntentType == IntentArticleScoped && err != nil && errors.Is(err, ErrArticleNotIndexed) {
		// Fallback stage 1: article-constrained general retrieval
		u.logger.Info("article_scoped_fallback_stage1",
			slog.String("article_id", intent.ArticleID),
			slog.String("reason", err.Error()))
		constrainedInput := retrieveInput
		constrainedInput.CandidateArticleIDs = []string{intent.ArticleID}
		retrieved, err = u.generalStrategy.Retrieve(ctx, constrainedInput, intent)
		result.strategyUsed = "article_constrained_fallback"

		if err != nil || (retrieved != nil && len(retrieved.Contexts) == 0) {
			// Fallback stage 2: unrestricted general (last resort)
			u.logger.Warn("unrestricted_fallback",
				slog.String("article_id", intent.ArticleID),
				slog.String("reason", "article_constrained_returned_empty"))
			retrieved, err = u.generalStrategy.Retrieve(ctx, retrieveInput, intent)
			result.strategyUsed = "unrestricted_general_fallback"
		}
	}

	if err != nil {
		return result, fmt.Errorf("failed to retrieve context: %w", err)
	}

	// Quality gate: assess retrieval quality and retry if marginal
	if u.qualityAssessor != nil && retrieved != nil && len(retrieved.Contexts) > 0 {
		verdict := u.qualityAssessor.Assess(retrieved.Contexts)
		result.retrievalQuality = verdict

		u.logger.Info("retrieval_quality_verdict",
			slog.String("retrieval_id", result.retrievalSetID),
			slog.String("verdict", string(verdict)),
			slog.String("strategy", result.strategyUsed))

		if verdict == QualityMarginal && u.queryExpander != nil {
			// Retry once with expanded query
			u.logger.Info("retrieval_quality_retry",
				slog.String("retrieval_id", result.retrievalSetID),
				slog.String("reason", "marginal_quality"))

			retryQuery := intent.UserQuestion
			expanded, expErr := u.queryExpander.ExpandQueryWithHistory(
				ctx, retryQuery, input.ConversationHistory, 2, 2,
			)
			if expErr == nil && len(expanded) > 0 {
				retryInput := retrieveInput
				retryInput.Query = expanded[0]
				retryRetrieved, retryErr := u.generalStrategy.Retrieve(ctx, retryInput, intent)
				if retryErr == nil && retryRetrieved != nil && len(retryRetrieved.Contexts) > 0 {
					retryVerdict := u.qualityAssessor.Assess(retryRetrieved.Contexts)
					if retryVerdict == QualityGood || (retryVerdict == QualityMarginal && verdict == QualityMarginal) {
						retrieved = retryRetrieved
						result.strategyUsed += "_retried"
						result.retrievalQuality = retryVerdict
					}
				}
			}
			result.retryCount = 1
		} else if verdict == QualityInsufficient {
			return result, errors.New("retrieval quality insufficient: context relevance too low")
		}
	}

	contexts := retrieved.Contexts
	originalContextCount := len(contexts)

	// Limit to maxChunks
	if len(contexts) > maxChunks {
		contexts = contexts[:maxChunks]
	}

	// Dynamic token-based limiting: prevent prompt from exceeding LLM context window.
	// Japanese text averages ~3 characters per token.
	maxPromptTokens := u.maxPromptTokens
	estimatedTokens := 500 // system prompt + query overhead
	var limitedContexts []ContextItem
	for _, ctx := range contexts {
		chunkTokens := len(ctx.ChunkText) / 3 // Japanese ~3 chars/token
		if estimatedTokens+chunkTokens > maxPromptTokens && len(limitedContexts) > 0 {
			break
		}
		estimatedTokens += chunkTokens
		limitedContexts = append(limitedContexts, ctx)
	}
	if len(limitedContexts) < len(contexts) {
		u.logger.Info("context_chunks_limited_by_tokens",
			slog.Int("original_count", len(contexts)),
			slog.Int("limited_count", len(limitedContexts)),
			slog.Int("estimated_tokens", estimatedTokens))
	}

	// Detect truncation for article-scoped context
	var artCtx *ArticleContext
	if intent.IntentType == IntentArticleScoped {
		truncated := len(limitedContexts) < len(contexts) || len(contexts) < originalContextCount
		if truncated {
			u.logger.Info("article_scoped_truncated",
				slog.String("article_id", intent.ArticleID),
				slog.Int("total_chunks", originalContextCount),
				slog.Int("used_chunks", len(limitedContexts)))
		}
		artCtx = &ArticleContext{
			ArticleID: intent.ArticleID,
			Title:     intent.ArticleTitle,
			Truncated: truncated,
		}
		result.articleContext = artCtx
	}
	contexts = limitedContexts

	result.contexts = contexts
	if retrieved.ExpandedQueries != nil {
		result.expandedQueries = retrieved.ExpandedQueries
	}

	if len(contexts) == 0 {
		return result, errors.New("no context returned from retrieval")
	}

	promptContexts := make([]PromptContext, len(contexts))
	for i, ctxItem := range contexts {
		promptContexts[i] = PromptContext{
			ChunkID:         ctxItem.ChunkID.String(),
			ChunkText:       ctxItem.ChunkText,
			Title:           ctxItem.Title,
			URL:             ctxItem.URL,
			PublishedAt:     ctxItem.PublishedAt,
			Score:           ctxItem.Score,
			DocumentVersion: ctxItem.DocumentVersion,
		}
	}

	locale := strings.TrimSpace(input.Locale)
	if locale == "" {
		locale = u.defaultLocale
	}

	// Phase 3: Tool dispatch (intent-driven, no LLM)
	var supplementary []string
	if u.toolDispatcher != nil {
		toolResults := u.toolDispatcher.Dispatch(ctx, intent, intent.UserQuestion)
		for _, tr := range toolResults {
			supplementary = append(supplementary, tr.Data)
			result.toolsUsed = append(result.toolsUsed, "tool")
		}
	}

	promptInput := PromptInput{
		Query:               intent.UserQuestion,
		Locale:              locale,
		PromptVersion:       u.promptVersion,
		Contexts:            promptContexts,
		ConversationHistory: input.ConversationHistory,
		ArticleContext:      artCtx,
		IntentType:          intent.IntentType,
		SupplementaryInfo:   supplementary,
	}

	messages, err := u.promptBuilder.Build(promptInput)
	if err != nil {
		return result, fmt.Errorf("failed to build messages: %v", err)
	}

	result.messages = messages
	return result, nil
}

func (u *answerWithRAGUsecase) toPromptContexts(contexts []ContextItem) []PromptContext {
	promptContexts := make([]PromptContext, len(contexts))
	for i, ctxItem := range contexts {
		promptContexts[i] = PromptContext{
			ChunkID:         ctxItem.ChunkID.String(),
			ChunkText:       ctxItem.ChunkText,
			Title:           ctxItem.Title,
			URL:             ctxItem.URL,
			PublishedAt:     ctxItem.PublishedAt,
			Score:           ctxItem.Score,
			DocumentVersion: ctxItem.DocumentVersion,
		}
	}
	return promptContexts
}

func (u *answerWithRAGUsecase) generateCacheKey(input AnswerWithRAGInput) string {
	// Normalize by sorting article IDs for consistent cache keys
	ids := make([]string, len(input.CandidateArticleIDs))
	copy(ids, input.CandidateArticleIDs)
	sort.Strings(ids)
	return fmt.Sprintf("%s|%v|%s", input.Query, ids, input.Locale)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// mergeContexts combines article-scoped chunks with general retrieval results.
// For follow-up re-retrieval, general results are placed FIRST because the
// follow-up question seeks NEW information beyond the article's own content.
// Article chunks are appended after as supplementary context.
// Deduplicates by chunk text prefix to avoid sending the same content twice.
func mergeContexts(article, general *RetrieveContextOutput) *RetrieveContextOutput {
	seen := make(map[string]bool, len(article.Contexts)+len(general.Contexts))
	merged := make([]ContextItem, 0, len(article.Contexts)+len(general.Contexts))

	// General results first — these are the NEW information from re-retrieval
	for _, ctx := range general.Contexts {
		key := dedupeKey(ctx)
		if !seen[key] {
			seen[key] = true
			merged = append(merged, ctx)
		}
	}
	// Article chunks after — supplementary context from the original article
	for _, ctx := range article.Contexts {
		key := dedupeKey(ctx)
		if !seen[key] {
			seen[key] = true
			merged = append(merged, ctx)
		}
	}

	result := *article
	result.Contexts = merged
	return &result
}

func dedupeKey(ctx ContextItem) string {
	text := ctx.ChunkText
	if len(text) > 80 {
		text = text[:80]
	}
	return ctx.URL + "|" + text
}
