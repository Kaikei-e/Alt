package qualitychecker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"pre-processor/config"
	"pre-processor/driver"
	"pre-processor/repository"
	"pre-processor/utils"
	logger "pre-processor/utils/logger"
)

// minValidScore/maxValidScore are the scale JudgeSystemPrompt asks the model for
// ("a single integer score from 1 to 10"), and the scale lowScoreThreshold is
// compared against. A score outside it is a broken response, not a verdict.
const (
	minValidScore = 1
	maxValidScore = 10
)

var (
	// qualityCheckerAPIURL can be overridden in tests
	qualityCheckerAPIURL = "http://news-creator:11434/api/generate"
	modelName            = "gemma4-e4b-12k"
	lowScoreThreshold    = 7 // 7 is the lowest score that is acceptable
	// Quality Checker用: 8kコンテキストモデルで安定動作する上限
	// 記事 + サマリー + プロンプトテンプレート(~1500文字) < 8k トークン
	// 日本語は約2文字/トークン（実測値）なので、記事+サマリーは約20,000文字まで
	maxQualityCheckContentLength = 20_000
	qualityCheckerHTTPTimeout    = 300 * time.Second

	httpClient = &http.Client{Timeout: qualityCheckerHTTPTimeout}

	// rateLimitHost/rateLimitInterval feed utils.NewsCreatorHostRateLimiter,
	// shared with the summarizer driver. news-creator is an internal service
	// protected by its own admission control; concurrency is bounded by the
	// summarize worker pool; the 5 s floor remains for external sites.
	rateLimitHost     = "http://news-creator:11434"
	rateLimitInterval = time.Duration(0)

	// knownPlaceholders are placeholder summaries saved when content is too short/long.
	// These must NOT be quality-checked: deleting them causes an infinite summarize→delete loop.
	knownPlaceholders = []string{
		"本文が短すぎるため要約できませんでした。",
		"本文が長すぎるため要約できませんでした。",
	}
)

// Configure overrides the quality checker's news-creator endpoint, model,
// threshold, content-length cap and rate limit from application config, and
// sends through client's transport (the news-creator mTLS client).
// Call once at startup (bootstrap/wire.go); the hardcoded package defaults
// above stay in effect for callers (and tests) that never call it.
func Configure(cfg *config.Config, client *http.Client) {
	if client == nil {
		panic("qualitychecker: Configure needs the news-creator HTTP client")
	}
	scoped := *client
	scoped.Timeout = qualityCheckerHTTPTimeout
	httpClient = &scoped

	if cfg == nil {
		return
	}
	if cfg.NewsCreator.Host != "" {
		rateLimitHost = cfg.NewsCreator.Host
		apiPath := cfg.QualityChecker.APIPath
		if apiPath == "" {
			apiPath = "/api/generate"
		}
		qualityCheckerAPIURL = cfg.NewsCreator.Host + apiPath
	}
	if cfg.QualityChecker.Model != "" {
		modelName = cfg.QualityChecker.Model
	}
	if cfg.QualityChecker.LowScoreThreshold > 0 {
		lowScoreThreshold = cfg.QualityChecker.LowScoreThreshold
	}
	if cfg.QualityChecker.MaxContentLength > 0 {
		maxQualityCheckContentLength = cfg.QualityChecker.MaxContentLength
	}
	if cfg.QualityChecker.Timeout > 0 {
		qualityCheckerHTTPTimeout = cfg.QualityChecker.Timeout
	}
	rateLimitInterval = cfg.NewsCreator.MinInterval
}

// isPlaceholderSummary returns true if the summary is a known placeholder message.
// Placeholder summaries are generated when article content is too short or too long
// for summarization. Quality-checking these would always yield low scores, causing
// deletion and re-summarization in an infinite loop.
func isPlaceholderSummary(summary string) bool {
	for _, placeholder := range knownPlaceholders {
		if summary == placeholder {
			return true
		}
	}
	return false
}

type judgePrompt struct {
	Model   string       `json:"model"`
	Prompt  string       `json:"prompt"`
	Options optionsModel `json:"options"`
	Stream  bool         `json:"stream"`
	Raw     bool         `json:"raw"`
}

type optionsModel struct {
	Stop        []string `json:"stop"`
	Temperature float64  `json:"temperature"`
	TopP        float64  `json:"top_p"`
	NumPredict  int      `json:"num_predict"`
	NumCtx      int      `json:"num_ctx"`
}

type ollamaResponse struct {
	Model      string `json:"model"`
	Response   string `json:"response"`
	DoneReason string `json:"done_reason"`
	Done       bool   `json:"done"`
}

type Score struct {
	Overall int `json:"overall"`
}

const JudgeSystemPrompt = `You are an expert Japanese news editor and quality evaluator. Your task is to rigorously assess the quality of a Japanese news summary against its source article.

ROLE:
You are a strict, objective quality grader specializing in Japanese news summarization. You evaluate summaries based on factual accuracy, completeness, readability, style, and information coverage.

TASK:
Read the provided Article (English source) and Summary (Japanese). Evaluate the Summary's quality across multiple dimensions and assign a single integer score from 1 to 10 (10 = best quality).

EVALUATION CRITERIA:
Evaluate the Summary based on these five dimensions:

1. ACCURACY (正確性):
   - Does the Summary accurately reflect facts from the Article?
   - Are there any factual errors, misrepresentations, or distortions?
   - Are numbers, dates, names, and key details correct?

2. COMPLETENESS (完全性):
   - Does the Summary include essential information: 5W1H (Who, What, When, Where, Why, How)?
   - Are there at least 2 numerical values (dates, amounts, counts, ratios)?
   - Are proper nouns included (with English notation on first mention if applicable)?
   - Are background/context, developments, and implications covered?

3. READABILITY (読みやすさ):
   - Is the Japanese natural and easy to understand?
   - Is the text coherent and well-structured?
   - Are there any awkward phrasings or unclear sentences?

4. STYLE (文体の適切性):
   - Is the style 常体（〜だ／である）consistently used?
   - Are there no headlines, bullet points, or unnecessary formatting?
   - Is the writing concise and appropriate for news style?

5. COVERAGE (情報の網羅性):
   - Does the Summary include: lead (most important facts + 5W1H), background/context, and implications/outlook?
   - Is the information balanced and comprehensive?

SCORING GUIDELINES:
- Score 1-3: Very poor quality (factual errors, missing critical information, poor readability)
- Score 4-6: Low quality (some missing information, style issues, incomplete coverage)
- Score 7-8: Acceptable quality (meets basic requirements, minor issues)
- Score 9-10: High quality (accurate, complete, readable, proper style, comprehensive coverage)

OUTPUT FORMAT:
- Output exactly one XML tag: <score>X</score>
- X must be an integer 1-10 (no decimals, no fractions)
- Do not output anything else: no explanations, no quotes, no extra tags, no preamble
- No leading/trailing spaces or newlines
- Do not output control tokens

CRITICAL RULES:
- Base your score on objective evaluation of all five criteria
- Be strict but fair: high scores (9-10) require excellence across all dimensions
- Low scores (1-3) indicate serious quality issues that would mislead readers
- Output ONLY the XML tag, nothing else`

const JudgeTemplate = `Article:
%s

Summary:
%s

Respond with only:
<score>X</score>`

var gemmaTurnTokens = []string{
	"<|think|>",
	"<|turn>",
	"<turn|>",
}

func stripGemmaTurnTokens(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	buf := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		b := s[i]
		buf = append(buf, b)
		if b != '>' {
			continue
		}
		for _, tok := range gemmaTurnTokens {
			if len(buf) >= len(tok) && string(buf[len(buf)-len(tok):]) == tok {
				buf = buf[:len(buf)-len(tok)]
				break
			}
		}
	}
	return string(buf)
}

func wrapGemmaPrompt(systemPrompt, userPrompt string) string {
	sys := strings.TrimSpace(stripGemmaTurnTokens(systemPrompt))
	user := strings.TrimSpace(stripGemmaTurnTokens(userPrompt))
	if sys != "" {
		return "<|turn>system\n" + sys + "\n<turn|>\n<|turn>user\n" + user + "\n<turn|>\n<|turn>model\n"
	}
	return "<|turn>user\n" + user + "\n<turn|>\n<|turn>model\n"
}

func buildJudgePrompt(articleContent, summaryJapanese string) string {
	userPrompt := fmt.Sprintf(JudgeTemplate, articleContent, summaryJapanese)
	return wrapGemmaPrompt(JudgeSystemPrompt, userPrompt)
}

func scoreSummary(ctx context.Context, prompt string) (*Score, error) {
	// Use more restrictive parameters to encourage format compliance
	opts := optionsModel{
		Temperature: 0.0,  // Very low temperature for more deterministic output
		TopP:        0.5,  // More restrictive sampling
		NumPredict:  60,   // Shorter to force concise responses
		NumCtx:      8192, // Match news-creator 8K model to prevent Ollama model reloads
		Stop:        []string{"</score>", "\n\n", "ARTICLE:", "SUMMARY:", "<turn|>", "<|user|>", "<|assistant|>"},
	}

	payload := judgePrompt{
		Model:   modelName,
		Prompt:  prompt,
		Stream:  false,
		Options: opts,
		Raw:     true, // buildJudgePrompt wraps the prompt in Gemma chat template tokens; prevent Ollama from double-applying them
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to marshal payload", "error", err)
		return nil, err
	}

	// news-creator is an internal service protected by its own admission
	// control; concurrency is bounded by the summarize worker pool; the 5 s
	// floor remains for external sites.
	if err := utils.NewsCreatorHostRateLimiter(rateLimitInterval).Wait(ctx, rateLimitHost); err != nil {
		logger.Logger.ErrorContext(ctx, "rate limiter wait failed", "error", err)
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", qualityCheckerAPIURL, strings.NewReader(string(jsonPayload)))
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to create HTTP request", "error", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	response, err := httpClient.Do(req)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to send HTTP request", "error", err)
		return nil, err
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			logger.Logger.ErrorContext(ctx, "failed to close response body", "error", err)
		}
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to read response body", "error", err)
		return nil, err
	}

	var ollamaResp ollamaResponse
	if err = json.Unmarshal(body, &ollamaResp); err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to unmarshal response", "error", err, "body", string(body))
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Received ollama response", "done", ollamaResp.Done, "response", ollamaResp.Response)

	if !ollamaResp.Done {
		logger.Logger.ErrorContext(ctx, "Ollama response not completed", "response", ollamaResp.Response)
		return nil, errors.New("ollama response not completed")
	}

	// Extract score from the response
	responseText := strings.TrimSpace(ollamaResp.Response)

	score, err := parseScore(responseText)
	if err != nil {
		// No fallback parsing. A malformed response must skip the article
		// rather than yield a fabricated score: anything below
		// lowScoreThreshold gets JudgeArticleQuality to delete a perfectly good
		// summary purely because the model's output format broke, and a broken
		// format tends to repeat, deleting summaries in a loop.
		logger.Logger.ErrorContext(ctx, "Failed to parse score, skipping quality check", "error", err, "response", responseText)
		return nil, fmt.Errorf("could not parse score from model response: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Successfully parsed score", "score", score)
	return &score, nil
}

// parseScore extracts the score from an anchored <score>N</score> tag and
// rejects everything else.
//
// There is deliberately no "find any integer" fallback. An unanchored integer
// is almost always part of the model's prose — 「3 つの事実が欠けている」 parsed
// as Score{Overall: 3}, below lowScoreThreshold, so a perfectly good summary was
// deleted and the article went back on the summarize queue. Because a broken
// output format repeats, that turned into a delete → re-summarize loop.
func parseScore(response string) (Score, error) {
	response = strings.TrimSpace(response)
	logger.Logger.Info("Parsing response", "original_response", response)

	// Closing tag is optional because Ollama's stop sequence includes "</score>",
	// causing it to stop generating before outputting the closing tag.
	//
	// Whitespace around the digits is tolerated: `<score> 8 </score>` and
	// `<score>\n8` are the model padding its own output, not prose, so rejecting
	// them only burns all three scoreSummaryWithRetry attempts and leaves the
	// article permanently unscoreable.
	//
	// The digits must still be followed by the closing tag or the end of the
	// reply. Without that, `<score>\n3 つの事実が欠けている。` parses as 3 — the
	// same prose adoption the removed fallback caused, just one tag further in.
	re := regexp.MustCompile(`<score>\s*(\d+)\s*(?:</score>|$)`)
	matches := re.FindStringSubmatch(response)

	if len(matches) != 2 {
		logger.Logger.Error("Could not extract score tag from response", "response", response)
		return Score{}, fmt.Errorf("could not extract <score> tag from response: %s", response)
	}

	scoreStr := matches[1]
	score, err := strconv.ParseInt(scoreStr, 10, strconv.IntSize)
	if err != nil {
		logger.Logger.Error("Failed to convert score to integer", "score_str", scoreStr, "error", err)
		return Score{}, fmt.Errorf("failed to convert score to integer: %w", err)
	}

	// Out-of-scale values are rejected, not clamped: clamping down deletes a
	// good summary and clamping up passes a bad one, both silently, when what
	// actually happened is that the model ignored the output contract.
	if score < minValidScore || score > maxValidScore {
		logger.Logger.Error("Score outside the scale requested by JudgeSystemPrompt",
			"score", score, "min", minValidScore, "max", maxValidScore)
		return Score{}, fmt.Errorf("score %d outside valid range %d-%d", score, minValidScore, maxValidScore)
	}

	return Score{Overall: int(score)}, nil
}

// RemoveLowScoreSummary deletes a low-quality summary via the repository.
// The score parameter is passed from the caller to avoid redundant LLM calls for scoring.
func RemoveLowScoreSummary(ctx context.Context, summaryRepo repository.SummaryRepository, articleRepo repository.ArticleRepository, articleWithSummary *driver.ArticleWithSummary, score *Score) error {
	if score == nil {
		logger.Logger.ErrorContext(ctx, "Received nil score", "articleID", articleWithSummary.ArticleID)
		return errors.New("received nil score for article " + articleWithSummary.ArticleID)
	}

	if summaryRepo == nil {
		return errors.New("summary repository is nil, cannot delete summary")
	}

	logger.Logger.InfoContext(ctx, "Removing low quality summary",
		"articleID", articleWithSummary.ArticleID,
		"score", score.Overall,
		"threshold", lowScoreThreshold)

	// Remove the summary via repository (works with both DB and API modes)
	err := summaryRepo.Delete(ctx, articleWithSummary.ArticleID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to delete article summary", "error", err, "articleID", articleWithSummary.ArticleID)
		return fmt.Errorf("failed to delete article summary: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Deleted low quality article summary", "articleID", articleWithSummary.ArticleID)

	return nil
}

// isConnectionError checks if an error is a connection-related error (network, timeout, context cancellation).
// This helps distinguish between actual service unavailability and low-quality scores.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context errors (timeout, cancellation)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Check for connection refused errors (common when service is down)
	errStr := err.Error()
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "i/o timeout") {
		return true
	}

	return false
}

// scoreSummaryWithRetry attempts to score a summary with retries and exponential backoff.
// It stops immediately on context cancellation instead of continuing to retry
// (and sleeping via a non-ctx-aware timer) against a caller that has already
// given up.
func scoreSummaryWithRetry(ctx context.Context, prompt string, maxRetries int) (*Score, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		score, err := scoreSummary(ctx, prompt)
		if err == nil && score != nil {
			return score, nil
		}
		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if attempt == maxRetries-1 {
			break
		}
		logger.Logger.WarnContext(ctx, "Failed to score summary, retrying...", "attempt", attempt+1, "max_retries", maxRetries, "error", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond): // Simple exponential backoff
		}
	}
	return nil, lastErr
}

// JudgeArticleQuality judges the quality of an article's summary and takes action if the score is low.
func JudgeArticleQuality(ctx context.Context, summaryRepo repository.SummaryRepository, articleRepo repository.ArticleRepository, articleWithSummary *driver.ArticleWithSummary) error {
	if articleWithSummary == nil || articleWithSummary.ArticleID == "" {
		return errors.New("article with summary is invalid")
	}

	// プレースホルダーサマリーはLLM評価不要 - 削除すると無限ループになる
	if isPlaceholderSummary(articleWithSummary.SummaryJapanese) {
		logger.Logger.InfoContext(ctx, "Skipping quality check: placeholder summary",
			"articleID", articleWithSummary.ArticleID)
		return nil
	}

	// コンテンツ長チェック: 長すぎる場合はスキップ（サマリーは保持）
	totalContentLength := len(articleWithSummary.Content) + len(articleWithSummary.SummaryJapanese)
	if totalContentLength > maxQualityCheckContentLength {
		logger.Logger.InfoContext(ctx, "Skipping quality check: content too long",
			"articleID", articleWithSummary.ArticleID,
			"content_length", len(articleWithSummary.Content),
			"summary_length", len(articleWithSummary.SummaryJapanese),
			"total_length", totalContentLength,
			"max_allowed", maxQualityCheckContentLength)
		return nil
	}

	prompt := buildJudgePrompt(articleWithSummary.Content, articleWithSummary.SummaryJapanese)
	score, err := scoreSummaryWithRetry(ctx, prompt, 3)
	if err != nil {
		if isConnectionError(err) {
			logger.Logger.WarnContext(ctx, "Connection error while scoring summary, skipping quality check to prevent data loss",
				"articleID", articleWithSummary.ArticleID,
				"error", err)
			return fmt.Errorf("failed to connect to news-creator service: %w", err)
		}

		logger.Logger.ErrorContext(ctx, "Failed to get summary score after retries (non-connection error)",
			"articleID", articleWithSummary.ArticleID,
			"error", err)
		return fmt.Errorf("failed to score summary (non-connection error): %w", err)
	}

	if score == nil {
		return errors.New("received nil score for article " + articleWithSummary.ArticleID)
	}

	// If score is too low, remove the summary (but keep the article)
	if score.Overall < lowScoreThreshold {
		return RemoveLowScoreSummary(ctx, summaryRepo, articleRepo, articleWithSummary, score)
	}

	logger.Logger.InfoContext(ctx, "Summary quality is acceptable", "articleID", articleWithSummary.ArticleID, "score", score.Overall)
	return nil
}
