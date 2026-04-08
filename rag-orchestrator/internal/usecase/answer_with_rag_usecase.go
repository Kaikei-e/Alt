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
	"rag-orchestrator/internal/usecase/retrieval"

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
	planner           *ConversationPlanner
	conversationStore *ConversationStore
	queryPlanner      domain.QueryPlannerPort
	relevanceGate     *RelevanceGate
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
	templateRegistry  *TemplateRegistry
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
	var tmplRegistry *TemplateRegistry
	if promptVersion == "alpha-v2" {
		tmplRegistry = NewTemplateRegistry()
	}
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
		planner:           cfg.planner,
		conversationStore: cfg.conversationStore,
		queryPlanner:      cfg.queryPlanner,
		relevanceGate:     cfg.relevanceGate,
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
		templateRegistry:  tmplRegistry,
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
	planner           *ConversationPlanner
	conversationStore *ConversationStore
	queryPlanner      domain.QueryPlannerPort
	relevanceGate     *RelevanceGate
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

// WithConversationPlanner enables conversation-aware planning before retrieval.
func WithConversationPlanner(planner *ConversationPlanner, store *ConversationStore) AnswerUsecaseOption {
	return func(cfg *answerUsecaseConfig) {
		cfg.planner = planner
		cfg.conversationStore = store
	}
}

// WithQueryPlanner enables LLM-based query planning via news-creator.
// When set, this replaces the rule-based ConversationPlanner and query expansion.
func WithQueryPlanner(qp domain.QueryPlannerPort) AnswerUsecaseOption {
	return func(cfg *answerUsecaseConfig) {
		cfg.queryPlanner = qp
	}
}

// WithRelevanceGate enables cross-encoder score-based quality gating.
func WithRelevanceGate(gate *RelevanceGate) AnswerUsecaseOption {
	return func(cfg *answerUsecaseConfig) {
		cfg.relevanceGate = gate
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
	finalPromptData, parsedAnswer, qualityFlags, generationRetryCount, accepted, err := u.generateAnswerWithRetries(ctx, input, promptData, requestID)
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

	if parsedAnswer.Fallback {
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", finalPromptData.retrievalSetID),
			slog.String("reason", parsedAnswer.Reason),
			slog.Int("contexts_available", len(finalPromptData.contexts)))
		return u.prepareFallback(
			finalPromptData.contexts,
			finalPromptData.retrievalSetID,
			parsedAnswer.Reason,
			FallbackLLMFallback,
			finalPromptData.strategyUsed,
			finalPromptData.expandedQueries,
		)
	}

	if !accepted {
		fallbackReason := selectFallbackReason(finalPromptData.intentType, qualityFlags)
		u.logger.Warn("answer_fallback_triggered",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", finalPromptData.retrievalSetID),
			slog.String("reason", fallbackReason),
			slog.Int("contexts_available", len(finalPromptData.contexts)))
		return u.prepareFallback(
			finalPromptData.contexts,
			finalPromptData.retrievalSetID,
			fallbackReason,
			FallbackShortUnderGrounded,
			finalPromptData.strategyUsed,
			finalPromptData.expandedQueries,
		)
	}

	// Build Citations (Hydration)
	finalCitations := u.buildCitations(finalPromptData.contexts, parsedAnswer.Citations)

	// Phase 4: Answer quality assessment
	if len(qualityFlags) > 0 {
		u.logger.Info("answer_quality_flags",
			slog.String("request_id", requestID),
			slog.Any("flags", qualityFlags))
	}

	debug := AnswerDebug{
		RetrievalSetID:        finalPromptData.retrievalSetID,
		PromptVersion:         u.promptVersion,
		ExpandedQueries:       finalPromptData.expandedQueries,
		StrategyUsed:          finalPromptData.strategyUsed,
		IntentType:            string(finalPromptData.intentType),
		SubIntentType:         string(finalPromptData.subIntentType),
		RetrievalQuality:      string(finalPromptData.retrievalQuality),
		RetryCount:            finalPromptData.retryCount + generationRetryCount,
		ToolsUsed:             finalPromptData.toolsUsed,
		QualityFlags:          qualityFlags,
		RetrievalPolicy:       finalPromptData.retrievalPolicy,
		GeneralRetrievalGated: finalPromptData.generalGated,
		BM25HitCount:          finalPromptData.bm25HitCount,
	}
	if finalPromptData.plannerOutput != nil {
		debug.PlannerOperation = string(finalPromptData.plannerOutput.Operation)
		debug.PlannerConfidence = finalPromptData.plannerOutput.Confidence
		debug.NeedsClarification = finalPromptData.plannerOutput.NeedsClarification
	}

	output := &AnswerWithRAGOutput{
		Answer:    strings.TrimSpace(parsedAnswer.Answer),
		Citations: finalCitations,
		Contexts:  finalPromptData.contexts,
		Fallback:  false,
		Reason:    "",
		Debug:     debug,
	}

	// 5. Store in Cache
	u.cache.Add(cacheKey, output)

	executionDuration := time.Since(executionStart)
	u.logger.Info("answer_request_completed",
		slog.String("request_id", requestID),
		slog.Int("answer_length", len(output.Answer)),
		slog.Int("citations", len(output.Citations)),
		slog.String("strategy_used", finalPromptData.strategyUsed),
		slog.Int64("total_duration_ms", executionDuration.Milliseconds()))

	return output, nil
}

type answerAcceptanceProfile struct {
	name                string
	maxRetries          int
	acceptMinRunes      int
	minCitations        int
	strictLongForm      bool
	rejectTruncatedJSON bool
}

func deriveAcceptanceProfile(input AnswerWithRAGInput, promptData *promptBuildResult) answerAcceptanceProfile {
	profile := answerAcceptanceProfile{
		name:                "default",
		maxRetries:          1,
		acceptMinRunes:      0,
		minCitations:        0,
		strictLongForm:      false,
		rejectTruncatedJSON: false,
	}
	if promptData == nil {
		return profile
	}

	isDetailed := queryRequestsDetailedAnswer(input.Query) ||
		promptData.intentType == IntentTopicDeepDive ||
		promptData.subIntentType == SubIntentDetail ||
		(promptData.plannerOutput != nil && promptData.plannerOutput.Operation == domain.OpDetail)
	if isDetailed {
		profile = answerAcceptanceProfile{
			name:                "detail",
			maxRetries:          2,
			acceptMinRunes:      240,
			minCitations:        2,
			strictLongForm:      true,
			rejectTruncatedJSON: true,
		}
	}
	if promptData.intentType == IntentSynthesis {
		profile = answerAcceptanceProfile{
			name:                "synthesis",
			maxRetries:          2,
			acceptMinRunes:      420,
			minCitations:        3,
			strictLongForm:      true,
			rejectTruncatedJSON: true,
		}
	}
	return profile
}

func (u *answerWithRAGUsecase) generateAnswerWithRetries(
	ctx context.Context,
	input AnswerWithRAGInput,
	promptData *promptBuildResult,
	requestID string,
) (*promptBuildResult, *LLMAnswer, []string, int, bool, error) {
	profile := deriveAcceptanceProfile(input, promptData)
	u.logger.Info("answer_acceptance_profile",
		slog.String("request_id", requestID),
		slog.String("profile", profile.name),
		slog.Int("max_retries", profile.maxRetries),
		slog.Int("accept_min_runes", profile.acceptMinRunes),
		slog.Int("min_citations", profile.minCitations))

	messages := promptData.messages
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

	retryCount := 0
	currentPromptData := promptData
	currentAnswer := (*LLMAnswer)(nil)
	currentFlags := []string(nil)

	for {
		generationStart := time.Now()
		u.logger.Info("llm_generation_started",
			slog.String("request_id", requestID),
			slog.String("retrieval_set_id", currentPromptData.retrievalSetID),
			slog.String("profile", profile.name),
			slog.Int("attempt", retryCount+1))

		resp, err := u.llmClient.Chat(ctx, currentPromptData.messages, currentPromptData.maxTokens)
		if err != nil {
			return currentPromptData, nil, currentFlags, retryCount, false, fmt.Errorf("generation failed: %w", err)
		}

		generationDuration := time.Since(generationStart)
		u.logger.Info("llm_generation_completed",
			slog.String("request_id", requestID),
			slog.String("profile", profile.name),
			slog.Int("attempt", retryCount+1),
			slog.Int("response_length", len(resp.Text)),
			slog.Int64("generation_ms", generationDuration.Milliseconds()))

		parsedAnswer, err := u.validator.Validate(resp.Text, currentPromptData.contexts)
		if err != nil {
			return currentPromptData, nil, currentFlags, retryCount, false, fmt.Errorf("validation failed: %w", err)
		}

		u.logger.Info("validation_completed",
			slog.String("request_id", requestID),
			slog.String("profile", profile.name),
			slog.Int("attempt", retryCount+1),
			slog.Bool("is_fallback", parsedAnswer.Fallback),
			slog.Int("citations_count", len(parsedAnswer.Citations)))

		if parsedAnswer.Fallback {
			return currentPromptData, parsedAnswer, currentFlags, retryCount, false, nil
		}

		qualityFlags := AssessAnswerQuality(
			parsedAnswer.Answer, input.Query, parsedAnswer.Citations, currentPromptData.intentType, currentPromptData.expandedQueries,
		)
		currentAnswer = parsedAnswer
		currentFlags = qualityFlags

		if len(qualityFlags) > 0 {
			u.logger.Info("answer_quality_flags",
				slog.String("request_id", requestID),
				slog.String("profile", profile.name),
				slog.Int("attempt", retryCount+1),
				slog.Any("flags", qualityFlags))
		}

		return u.retryValidatedAnswer(ctx, input, currentPromptData, currentAnswer, currentFlags, profile, requestID, retryCount)
	}
}

func (u *answerWithRAGUsecase) retryValidatedAnswer(
	ctx context.Context,
	input AnswerWithRAGInput,
	promptData *promptBuildResult,
	parsedAnswer *LLMAnswer,
	qualityFlags []string,
	profile answerAcceptanceProfile,
	requestID string,
	retryCount int,
) (*promptBuildResult, *LLMAnswer, []string, int, bool, error) {
	currentPromptData := promptData
	currentAnswer := parsedAnswer
	currentFlags := qualityFlags

	for {
		if u.answerAccepted(profile, currentAnswer, currentFlags) && !currentAnswer.ShortAnswer {
			return currentPromptData, currentAnswer, currentFlags, retryCount, true, nil
		}

		if currentAnswer.ShortAnswer {
			u.logger.Warn("short_answer_detected",
				slog.String("request_id", requestID),
				slog.String("retrieval_set_id", currentPromptData.retrievalSetID),
				slog.String("profile", profile.name),
				slog.Int("answer_rune_length", utf8.RuneCountInString(currentAnswer.Answer)),
				slog.String("query", input.Query),
				slog.Any("quality_flags", currentFlags))
		}

		if retryCount >= profile.maxRetries {
			accepted := u.answerAccepted(profile, currentAnswer, currentFlags)
			if accepted && currentAnswer.ShortAnswer {
				if hasQualityFlag(currentFlags, "low_keyword_coverage") ||
					hasQualityFlag(currentFlags, "incoherent_ending") ||
					hasQualityFlag(currentFlags, "context_insufficiency_disclaimer") {
					accepted = false
				}
			}
			return currentPromptData, currentAnswer, currentFlags, retryCount, accepted, nil
		}
		if !u.shouldRetryGeneratedAnswer(input.Query, currentAnswer, currentPromptData, currentFlags, profile) {
			return currentPromptData, currentAnswer, currentFlags, retryCount, u.answerAccepted(profile, currentAnswer, currentFlags), nil
		}

		retryInput := u.buildCorrectiveRetryInput(input, promptData, profile, retryCount+1)
		retryPromptData, err := u.buildPrompt(ctx, retryInput)
		if err != nil {
			return currentPromptData, parsedAnswer, qualityFlags, retryCount, false, fmt.Errorf("build corrective retry prompt: %w", err)
		}

		retryResp, err := u.llmClient.Chat(ctx, retryPromptData.messages, retryPromptData.maxTokens)
		if err != nil {
			return currentPromptData, parsedAnswer, qualityFlags, retryCount, false, fmt.Errorf("corrective retry generation failed: %w", err)
		}

		retryParsed, err := u.validator.Validate(retryResp.Text, retryPromptData.contexts)
		if err != nil {
			return currentPromptData, parsedAnswer, qualityFlags, retryCount, false, fmt.Errorf("corrective retry validation failed: %w", err)
		}

		retryFlags := AssessAnswerQuality(
			retryParsed.Answer, input.Query, retryParsed.Citations, retryPromptData.intentType, retryPromptData.expandedQueries,
		)
		retryCount++
		if shouldKeepOriginalAfterRetry(currentAnswer, currentFlags, retryParsed, retryFlags) {
			u.logger.Warn("corrective_retry_discarded_degraded_answer",
				slog.String("request_id", requestID),
				slog.String("profile", profile.name),
				slog.String("original_retrieval_set_id", currentPromptData.retrievalSetID),
				slog.String("retry_retrieval_set_id", retryPromptData.retrievalSetID),
				slog.Any("retry_flags", retryFlags))
			continue
		}

		currentPromptData = retryPromptData
		currentAnswer = retryParsed
		currentFlags = retryFlags
		if u.answerAccepted(profile, currentAnswer, currentFlags) && !currentAnswer.ShortAnswer {
			return currentPromptData, currentAnswer, currentFlags, retryCount, true, nil
		}
	}
}

func (u *answerWithRAGUsecase) shouldRetryGeneratedAnswer(
	query string,
	parsedAnswer *LLMAnswer,
	promptData *promptBuildResult,
	qualityFlags []string,
	profile answerAcceptanceProfile,
) bool {
	if parsedAnswer == nil || promptData == nil {
		return false
	}
	answerLen := utf8.RuneCountInString(parsedAnswer.Answer)
	if hasQualityFlag(qualityFlags, "low_keyword_coverage") ||
		hasQualityFlag(qualityFlags, "low_citation_density") ||
		hasQualityFlag(qualityFlags, "incoherent_ending") ||
		hasQualityFlag(qualityFlags, "expansion_failed") ||
		hasQualityFlag(qualityFlags, "context_insufficiency_disclaimer") ||
		len(parsedAnswer.Citations) == 0 {
		return true
	}
	if promptData.intentType == IntentCausalExplanation && answerLen < 160 {
		return true
	}
	if profile.strictLongForm && answerLen < profile.acceptMinRunes {
		return true
	}
	return queryRequestsDetailedAnswer(query) && answerLen < profile.acceptMinRunes
}

func (u *answerWithRAGUsecase) answerAccepted(profile answerAcceptanceProfile, parsedAnswer *LLMAnswer, qualityFlags []string) bool {
	if parsedAnswer == nil || parsedAnswer.Fallback {
		return false
	}
	if hasQualityFlag(qualityFlags, "context_insufficiency_disclaimer") {
		return false
	}
	if len(parsedAnswer.Citations) < profile.minCitations {
		return false
	}
	if profile.strictLongForm {
		answerLen := utf8.RuneCountInString(parsedAnswer.Answer)
		if answerLen < profile.acceptMinRunes {
			return false
		}
		if profile.rejectTruncatedJSON && parsedAnswer.Reason == "recovered_from_truncated_json" {
			return false
		}
	}
	return true
}

func (u *answerWithRAGUsecase) buildCorrectiveRetryInput(
	input AnswerWithRAGInput,
	promptData *promptBuildResult,
	profile answerAcceptanceProfile,
	attempt int,
) AnswerWithRAGInput {
	retryInput := input

	baseTokens := promptData.maxTokens
	if baseTokens <= 0 {
		baseTokens = u.maxTokens
	}
	retryInput.MaxTokens = baseTokens + max(256, baseTokens/2)

	retryHint := "より詳しく、直接原因、背景要因、影響、未確定点を分けて説明してください。"
	if promptData.intentType == IntentCausalExplanation {
		retryHint = "因果関係をより詳しく、直接原因、背景要因、構造要因、影響を分けて説明してください。"
	}
	if profile.name == "detail" {
		if promptData.intentType == IntentCausalExplanation {
			if attempt == 1 {
				retryHint = "因果関係を短くまとめず、複数段落で直接原因、背景要因、構造要因、影響を分けて詳しく説明してください。"
			} else {
				retryHint = "最低4段落で、直接原因、背景要因、構造要因、影響、不確実性を分けて詳しく説明してください。"
			}
		} else if attempt == 1 {
			retryHint = "短くまとめず、複数段落で具体的な事実と引用を交えて説明してください。"
		} else {
			retryHint = "最低4段落で、背景、直接要因、構造要因、影響、不確実性を分けて詳しく説明してください。"
		}
	} else if profile.name == "synthesis" {
		if attempt == 1 {
			retryHint = "3つ以上の観点を分け、各観点に具体例と引用を付けて包括的に説明してください。"
		} else {
			retryHint = "最低4段落で、相互関係・現状・展望・不確実性を分けて詳しく説明してください。"
		}
	} else if queryRequestsDetailedAnswer(input.Query) {
		retryHint += " 短くまとめず、複数段落で具体的な事実と引用を交えて説明してください。"
	}
	if strings.TrimSpace(retryInput.Query) != "" && !strings.Contains(retryInput.Query, retryHint) {
		retryInput.Query = strings.TrimSpace(retryInput.Query) + "\n\n" + retryHint
	}

	return retryInput
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func shouldKeepOriginalAfterRetry(
	original *LLMAnswer,
	originalFlags []string,
	retry *LLMAnswer,
	retryFlags []string,
) bool {
	if original == nil || retry == nil {
		return false
	}
	if !hasQualityFlag(retryFlags, "context_insufficiency_disclaimer") {
		return false
	}
	if hasQualityFlag(originalFlags, "context_insufficiency_disclaimer") ||
		hasQualityFlag(originalFlags, "low_keyword_coverage") ||
		hasQualityFlag(originalFlags, "incoherent_ending") {
		return false
	}
	return len(original.Citations) > 0
}

func queryRequestsDetailedAnswer(query string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(query))
	if trimmed == "" {
		return false
	}

	detailSignals := []string{
		"詳しく",
		"詳細",
		"くわしく",
		"背景も",
		"背景まで",
		"深く",
		"in detail",
		"detailed",
		"more detail",
		"explain fully",
	}
	for _, signal := range detailSignals {
		if strings.Contains(trimmed, signal) {
			return true
		}
	}
	return false
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
	subIntentType    SubIntentType
	toolsUsed        []string
	articleContext   *ArticleContext
	retrievalQuality QualityVerdict
	retryCount       int
	retrievalPolicy  string
	generalGated     bool
	plannerOutput    *domain.PlannerOutput // Conversation planner result
	parsedIntent     QueryIntent           // Resolved intent for state derivation
	bm25HitCount     int
	lowConfidence    bool // Insufficient quality but generating with disclaimer
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

	// --- LLM-based query planner (P0-4) ---
	// When queryPlanner is configured, it replaces the legacy
	// ResolveQueryIntent + QueryClassifier + ConversationPlanner pipeline.
	if u.queryPlanner != nil {
		return u.buildPromptWithQueryPlanner(ctx, input, result)
	}

	// --- Legacy path (will be removed in P1-3) ---

	// Parse intent from raw query
	intent := ResolveQueryIntent(input.Query, input.ConversationHistory)

	// Smart classification
	if u.queryClassifier != nil {
		if intent.IntentType == IntentArticleScoped {
			// Sub-classify the actual question within article scope
			intent.SubIntentType = u.queryClassifier.ClassifySubIntent(intent.UserQuestion)
		} else {
			// Non-article-scoped: use full classifier for richer intent
			classified := u.queryClassifier.Classify(ctx, intent.UserQuestion)
			if classified != IntentGeneral {
				intent.IntentType = classified
			}
		}
	}

	strategy := u.selectStrategy(intent.IntentType)
	result.strategyUsed = strategy.Name()
	result.intentType = intent.IntentType
	result.subIntentType = intent.SubIntentType
	result.parsedIntent = intent

	u.logger.Info("query_intent_parsed",
		slog.String("intent_type", string(intent.IntentType)),
		slog.String("sub_intent_type", string(intent.SubIntentType)),
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

	// Conversation planner: resolve ambiguity and determine retrieval policy.
	var plan *domain.PlannerOutput
	if u.planner != nil {
		var convState *domain.ConversationState
		if u.conversationStore != nil {
			convState = u.conversationStore.Get(input.UserID)
		}
		plan = u.planner.Plan(intent.UserQuestion, intent, convState, input.ConversationHistory)
		result.plannerOutput = plan

		u.logger.Info("planner_output",
			slog.String("operation", string(plan.Operation)),
			slog.String("retrieval_policy", string(plan.RetrievalPolicy)),
			slog.Float64("confidence", plan.Confidence),
			slog.Bool("needs_clarification", plan.NeedsClarification))
	}

	// Retrieve contexts using planner-driven or legacy policy.
	retrieved, err := u.retrieveWithPolicy(ctx, strategy, retrieveInput, intent, plan, input, result)

	// Legacy path: when no planner, use existing sub-intent policy switch.
	if plan == nil && intent.IntentType == IntentArticleScoped && len(input.ConversationHistory) > 0 && err == nil && retrieved != nil {
		retrieved = u.applyLegacySubIntentPolicy(ctx, intent, input, retrieved, result)
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

	// Quality gate: assess retrieval quality with intent-aware strictness
	if u.qualityAssessor != nil && retrieved != nil && len(retrieved.Contexts) > 0 {
		verdict := u.qualityAssessor.AssessWithIntent(retrieved.Contexts, intent.IntentType, intent.UserQuestion)
		result.retrievalQuality = verdict

		u.logger.Info("retrieval_quality_verdict",
			slog.String("retrieval_id", result.retrievalSetID),
			slog.String("verdict", string(verdict)),
			slog.String("strategy", result.strategyUsed))

		if verdict == QualityMarginal && u.queryExpander != nil {
			// Retry with expanded/decomposed queries.
			// For causal queries, use multiple focused subqueries instead of a single
			// broad rewrite. This follows Azure RAG guidance: "decomposed subqueries"
			// preserve the original query's intent while narrowing retrieval focus.
			u.logger.Info("retrieval_quality_retry",
				slog.String("retrieval_id", result.retrievalSetID),
				slog.String("reason", "marginal_quality"),
				slog.String("intent_type", string(intent.IntentType)))

			retryQueries := u.buildRetryQueries(ctx, intent, input.ConversationHistory)

			var bestRetrieved *RetrieveContextOutput
			var bestVerdict QualityVerdict
			for _, rq := range retryQueries {
				retryInput := retrieveInput
				retryInput.Query = rq
				retryRetrieved, retryErr := u.generalStrategy.Retrieve(ctx, retryInput, intent)
				if retryErr != nil || retryRetrieved == nil || len(retryRetrieved.Contexts) == 0 {
					continue
				}
				retryVerdict := u.qualityAssessor.AssessWithIntent(retryRetrieved.Contexts, intent.IntentType, intent.UserQuestion)
				if retryVerdict == QualityGood || (bestRetrieved == nil && retryVerdict == QualityMarginal) {
					bestRetrieved = retryRetrieved
					bestVerdict = retryVerdict
				}
				if retryVerdict == QualityGood {
					break // Found good result, stop trying
				}
			}
			if bestRetrieved != nil {
				retrieved = bestRetrieved
				result.strategyUsed += "_retried"
				result.retrievalQuality = bestVerdict
			}
			result.retryCount = len(retryQueries)
		} else if verdict == QualityInsufficient {
			// Graceful degradation: generate with low-confidence disclaimer
			// instead of hard fallback. Google ICLR 2025 shows models can
			// correctly answer 35-62% of queries even with insufficient context.
			result.lowConfidence = true
			u.logger.Warn("retrieval_quality_low_confidence",
				slog.String("retrieval_id", result.retrievalSetID),
				slog.String("reason", "insufficient_quality_generating_with_disclaimer"))
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
	estimatedTokens := EstimateSystemPromptTokens(u.promptVersion, intent.IntentType, u.templateRegistry)
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

	// Allow empty contexts when tool results are the primary content or
	// when the planner determined no retrieval is needed (clarification, tool-only).
	allowEmpty := intent.SubIntentType == SubIntentRelatedArticles
	if plan != nil && (plan.RetrievalPolicy == domain.PolicyToolOnly || plan.RetrievalPolicy == domain.PolicyNoRetrieval) {
		allowEmpty = true
	}
	if len(contexts) == 0 && !allowEmpty {
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

	// For synthesis strategy, tool results are already in the retrieval output
	if retrieved != nil && len(retrieved.SupplementaryInfo) > 0 {
		supplementary = append(supplementary, retrieved.SupplementaryInfo...)
	}
	if retrieved != nil && len(retrieved.ToolsUsed) > 0 {
		result.toolsUsed = append(result.toolsUsed, retrieved.ToolsUsed...)
	}

	// Additional tool dispatch for non-synthesis intents
	if u.toolDispatcher != nil && intent.IntentType != IntentSynthesis {
		toolResults := u.toolDispatcher.Dispatch(ctx, intent, intent.UserQuestion)
		for _, tr := range toolResults {
			supplementary = append(supplementary, tr.Data)
			result.toolsUsed = append(result.toolsUsed, tr.ToolName)
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
		SubIntentType:       intent.SubIntentType,
		SupplementaryInfo:   supplementary,
		LowConfidence:       result.lowConfidence,
	}

	messages, err := u.promptBuilder.Build(promptInput)
	if err != nil {
		return result, fmt.Errorf("build messages: %w", err)
	}

	result.messages = messages
	return result, nil
}

// retrieveWithPolicy executes retrieval based on planner output policy.
// When no planner is configured, falls back to the standard strategy.Retrieve path.
func (u *answerWithRAGUsecase) retrieveWithPolicy(
	ctx context.Context,
	strategy RetrievalStrategy,
	retrieveInput RetrieveContextInput,
	intent QueryIntent,
	plan *domain.PlannerOutput,
	input AnswerWithRAGInput,
	result *promptBuildResult,
) (*RetrieveContextOutput, error) {
	if plan == nil {
		return strategy.Retrieve(ctx, retrieveInput, intent)
	}

	generalInput := RetrieveContextInput{
		Query:               intent.UserQuestion,
		ConversationHistory: input.ConversationHistory,
	}

	switch plan.RetrievalPolicy {
	case domain.PolicyArticleOnly:
		result.retrievalPolicy = "article_only"
		result.generalGated = true
		return strategy.Retrieve(ctx, retrieveInput, intent)

	case domain.PolicyArticlePlusGlobal:
		result.retrievalPolicy = "article_plus_global"
		retrieved, err := strategy.Retrieve(ctx, retrieveInput, intent)
		if err != nil {
			return retrieved, fmt.Errorf("retrieve with article_plus_global policy: %w", err)
		}
		if u.qualityAssessor != nil && retrieved != nil {
			verdict := u.qualityAssessor.Assess(retrieved.Contexts)
			if verdict != QualityGood {
				generalResult, genErr := u.generalStrategy.Retrieve(ctx, generalInput, intent)
				if genErr == nil && generalResult != nil && len(generalResult.Contexts) > 0 {
					retrieved = mergeContexts(retrieved, generalResult)
					result.strategyUsed = strategy.Name() + "+general"
				}
			}
		}
		return retrieved, nil

	case domain.PolicyGlobalOnly:
		result.retrievalPolicy = "global_only"
		// Use the intent-selected strategy (e.g. SynthesisStrategy for IntentSynthesis)
		// rather than always falling back to generalStrategy.
		return strategy.Retrieve(ctx, retrieveInput, intent)

	case domain.PolicyToolOnly:
		result.retrievalPolicy = "tool_only"
		result.generalGated = true
		// Tool-only: return empty contexts; tool dispatch later supplies data.
		return &RetrieveContextOutput{Contexts: nil}, nil

	case domain.PolicyNoRetrieval:
		result.retrievalPolicy = "no_retrieval"
		return &RetrieveContextOutput{Contexts: nil}, nil

	default:
		return strategy.Retrieve(ctx, retrieveInput, intent)
	}
}

// applyLegacySubIntentPolicy preserves the existing sub-intent-driven retrieval
// policy for backward compatibility when no ConversationPlanner is configured.
// Returns the (potentially merged) retrieved output.
func (u *answerWithRAGUsecase) applyLegacySubIntentPolicy(
	ctx context.Context,
	intent QueryIntent,
	input AnswerWithRAGInput,
	retrieved *RetrieveContextOutput,
	result *promptBuildResult,
) *RetrieveContextOutput {
	switch intent.SubIntentType {
	case SubIntentDetail, SubIntentEvidence, SubIntentSummaryRefresh:
		u.logger.Info("retrieval_policy_article_only",
			slog.String("sub_intent", string(intent.SubIntentType)),
			slog.String("article_id", intent.ArticleID))
		result.retrievalPolicy = "article_only"
		result.generalGated = true

	case SubIntentRelatedArticles:
		u.logger.Info("retrieval_policy_tool_delegated",
			slog.String("sub_intent", string(intent.SubIntentType)),
			slog.String("article_id", intent.ArticleID))
		result.retrievalPolicy = "tool_delegated"
		result.generalGated = true

	case SubIntentCritique, SubIntentOpinion, SubIntentImplication:
		result.retrievalPolicy = "article_first_analytical"
		result.generalGated = true
		if u.qualityAssessor != nil {
			verdict := u.qualityAssessor.Assess(retrieved.Contexts)
			if verdict == QualityMarginal || verdict == QualityInsufficient {
				u.logger.Info("analytical_subintent_general_augmentation",
					slog.String("sub_intent", string(intent.SubIntentType)),
					slog.String("verdict", string(verdict)))
				generalInput := RetrieveContextInput{
					Query:               intent.UserQuestion,
					ConversationHistory: input.ConversationHistory,
				}
				generalResult, genErr := u.generalStrategy.Retrieve(ctx, generalInput, intent)
				if genErr == nil && generalResult != nil && len(generalResult.Contexts) > 0 {
					before := len(retrieved.Contexts)
					retrieved = mergeContexts(retrieved, generalResult)
					u.logger.Info("analytical_general_merge",
						slog.Int("article_chunks", before),
						slog.Int("general_chunks", len(generalResult.Contexts)),
						slog.Int("merged_total", len(retrieved.Contexts)))
					result.strategyUsed = "article_scoped+general_analytical"
				}
			}
		}

	default:
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
	return retrieved
}

// buildRetryQueries generates focused retry queries based on intent type.
// For causal queries, decomposes the question into focused subqueries
// targeting different causal aspects (supply, geopolitical, economic).
// For other intents, uses the standard query expansion approach.
func (u *answerWithRAGUsecase) buildRetryQueries(
	ctx context.Context,
	intent QueryIntent,
	history []domain.Message,
) []string {
	if intent.IntentType == IntentCausalExplanation {
		// Causal decomposition: focused subqueries for different causal aspects
		base := intent.UserQuestion
		return []string{
			base + " 供給 制裁 sanctions supply",
			base + " 地政学 geopolitical conflict",
			base + " 経済 market price impact",
		}
	}

	// Default: use query expander for a single retry
	if u.queryExpander != nil {
		expanded, err := u.queryExpander.ExpandQueryWithHistory(
			ctx, intent.UserQuestion, history, 2, 2,
		)
		if err == nil && len(expanded) > 0 {
			return expanded[:1]
		}
	}
	return nil
}

// buildPromptWithQueryPlanner uses the LLM-based QueryPlannerPort to plan retrieval.
// This replaces the legacy ResolveQueryIntent + QueryClassifier + ConversationPlanner.
func (u *answerWithRAGUsecase) buildPromptWithQueryPlanner(
	ctx context.Context,
	input AnswerWithRAGInput,
	result *promptBuildResult,
) (*promptBuildResult, error) {
	// Call the query planner
	qpInput := domain.QueryPlannerInput{
		Query:               input.Query,
		ConversationHistory: input.ConversationHistory,
	}

	// Extract article scope from the raw query (reuse ParseQueryIntent for metadata extraction)
	parsedIntent := ParseQueryIntent(input.Query)
	if parsedIntent.IntentType == IntentArticleScoped {
		qpInput.ArticleID = parsedIntent.ArticleID
		qpInput.ArticleTitle = parsedIntent.ArticleTitle
		qpInput.Query = parsedIntent.UserQuestion // Strip article metadata
	}

	qPlan, err := u.queryPlanner.PlanQuery(ctx, qpInput)
	if err != nil {
		u.logger.Warn("query_planner_failed_falling_back",
			slog.String("error", err.Error()),
			slog.String("query", input.Query))
		// Fallback: use original query directly
		qPlan = &domain.QueryPlan{
			ResolvedQuery:   qpInput.Query,
			SearchQueries:   []string{qpInput.Query},
			Intent:          "general",
			RetrievalPolicy: "global_only",
			AnswerFormat:    "summary",
		}
	}

	u.logger.Info("query_planner_output",
		slog.String("resolved_query", qPlan.ResolvedQuery),
		slog.String("intent", qPlan.Intent),
		slog.String("retrieval_policy", qPlan.RetrievalPolicy),
		slog.Bool("should_clarify", qPlan.ShouldClarify),
		slog.Int("search_queries", len(qPlan.SearchQueries)))

	// Map plan to result metadata
	result.intentType = IntentType(qPlan.Intent)
	result.retrievalPolicy = qPlan.RetrievalPolicy
	result.expandedQueries = retrieval.FilterSearchQueries(qPlan.SearchQueries, qPlan.ResolvedQuery)
	plannerIntent := parsedIntent
	plannerIntent.IntentType = result.intentType
	plannerIntent.SearchQueries = result.expandedQueries
	if strings.TrimSpace(qPlan.ResolvedQuery) != "" {
		plannerIntent.UserQuestion = qPlan.ResolvedQuery
	}

	// Map to PlannerOutput for compatibility with stream clarification
	plannerOut := &domain.PlannerOutput{
		Operation:          domain.PlannerOperation(qPlan.Intent),
		RetrievalPolicy:    domain.RetrievalPolicy(qPlan.RetrievalPolicy),
		NeedsClarification: qPlan.ShouldClarify,
		ClarificationMsg:   qPlan.ClarificationMsg,
		Confidence:         0.8, // LLM planner confidence
	}
	result.plannerOutput = plannerOut

	// Clarification: short-circuit before retrieval
	if qPlan.ShouldClarify {
		result.contexts = nil
		return result, nil
	}

	// Select strategy based on LLM-classified intent
	strategy := u.selectStrategy(result.intentType)
	result.strategyUsed = strategy.Name()

	// Use resolved query for retrieval
	retrieveInput := RetrieveContextInput{
		Query:               qPlan.ResolvedQuery,
		ConversationHistory: input.ConversationHistory,
	}
	if len(input.CandidateArticleIDs) > 0 {
		retrieveInput.CandidateArticleIDs = input.CandidateArticleIDs
	}

	// Retrieve with the planner's policy
	retrieved, err := u.retrieveWithPolicy(ctx, strategy, retrieveInput, plannerIntent, plannerOut, input, result)
	if err != nil {
		return result, fmt.Errorf("failed to retrieve context: %w", err)
	}
	if retrieved == nil {
		return result, errors.New("no context returned from retrieval")
	}

	// Quality gate: prefer RelevanceGate (cross-encoder score based),
	// fall back to legacy heuristic assessor.
	if retrieved != nil && len(retrieved.Contexts) > 0 {
		var verdict QualityVerdict
		if u.relevanceGate != nil {
			verdict = u.relevanceGate.Evaluate(retrieved.Contexts)
		} else if u.qualityAssessor != nil {
			verdict = u.qualityAssessor.AssessWithIntent(retrieved.Contexts, result.intentType, qPlan.ResolvedQuery)
		}
		result.retrievalQuality = verdict

		if verdict == QualityInsufficient {
			return result, errors.New("retrieval quality insufficient: context relevance too low")
		}
	}

	contexts := retrieved.Contexts
	if len(contexts) > u.maxChunks {
		contexts = contexts[:u.maxChunks]
	}

	// Token-based limiting
	maxPromptTokens := u.maxPromptTokens
	estimatedTokens := EstimateSystemPromptTokens(u.promptVersion, result.intentType, u.templateRegistry)
	var limitedContexts []ContextItem
	for _, ctx := range contexts {
		chunkTokens := len(ctx.ChunkText) / 3
		if estimatedTokens+chunkTokens > maxPromptTokens && len(limitedContexts) > 0 {
			break
		}
		estimatedTokens += chunkTokens
		limitedContexts = append(limitedContexts, ctx)
	}
	contexts = limitedContexts

	result.contexts = contexts
	if retrieved.ExpandedQueries != nil {
		result.expandedQueries = retrieved.ExpandedQueries
	}

	if len(contexts) == 0 && qPlan.RetrievalPolicy != "tool_only" && qPlan.RetrievalPolicy != "no_retrieval" {
		return result, errors.New("no context returned from retrieval")
	}

	// Build prompt messages
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

	// Tool dispatch (supplementary info)
	var supplementary []string
	if retrieved != nil && len(retrieved.SupplementaryInfo) > 0 {
		supplementary = append(supplementary, retrieved.SupplementaryInfo...)
	}
	if retrieved != nil && len(retrieved.ToolsUsed) > 0 {
		result.toolsUsed = append(result.toolsUsed, retrieved.ToolsUsed...)
	}
	if u.toolDispatcher != nil {
		toolResults := u.toolDispatcher.Dispatch(ctx, parsedIntent, qPlan.ResolvedQuery)
		for _, tr := range toolResults {
			supplementary = append(supplementary, tr.Data)
			result.toolsUsed = append(result.toolsUsed, tr.ToolName)
		}
	}

	promptInput := PromptInput{
		Query:               qPlan.ResolvedQuery,
		Locale:              locale,
		PromptVersion:       u.promptVersion,
		Contexts:            promptContexts,
		ConversationHistory: input.ConversationHistory,
		IntentType:          result.intentType,
		SupplementaryInfo:   supplementary,
		PlannerOutput:       plannerOut,
	}

	messages, err := u.promptBuilder.Build(promptInput)
	if err != nil {
		return result, fmt.Errorf("build messages: %w", err)
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
