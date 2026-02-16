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

	"pre-processor/driver"
	"pre-processor/repository"
	logger "pre-processor/utils/logger"
)

var (
	// qualityCheckerAPIURL can be overridden in tests
	qualityCheckerAPIURL = "http://news-creator:11434/api/generate"
	modelName            = "gemma3-4b-8k"
	lowScoreThreshold    = 7 // 7 is the lowest score that is acceptable
	// Quality Checker用: 8kコンテキストモデルで安定動作する上限
	// 記事 + サマリー + プロンプトテンプレート(~1500文字) < 8k トークン
	// 日本語は約2文字/トークン（実測値）なので、記事+サマリーは約20,000文字まで
	maxQualityCheckContentLength = 20_000
)

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

const JudgeTemplate = `
<start_of_turn>user
You are an expert Japanese news editor and quality evaluator. Your task is to rigorously assess the quality of a Japanese news summary against its source article.

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
- Do not output control tokens (e.g., <end_of_turn>)

CRITICAL RULES:
- Base your score on objective evaluation of all five criteria
- Be strict but fair: high scores (9-10) require excellence across all dimensions
- Low scores (1-3) indicate serious quality issues that would mislead readers
- Output ONLY the XML tag, nothing else

Article:
%s

Summary:
%s

Respond with only:
<score>X</score>
<end_of_turn>
<start_of_turn>model

`

func scoreSummary(ctx context.Context, prompt string) (*Score, error) {
	// Use more restrictive parameters to encourage format compliance
	opts := optionsModel{
		Temperature: 0.0,  // Very low temperature for more deterministic output
		TopP:        0.5,  // More restrictive sampling
		NumPredict:  60,   // Shorter to force concise responses
		NumCtx:      8192, // Match news-creator 8K model to prevent Ollama model reloads
		Stop:        []string{"</score>", "\n\n", "ARTICLE:", "SUMMARY:", "<|user|>", "<|assistant|>"},
	}

	payload := judgePrompt{
		Model:   modelName,
		Prompt:  prompt,
		Stream:  false,
		Options: opts,
		Raw:     true, // JudgeTemplate contains Gemma chat template tokens; prevent Ollama from double-applying them
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to marshal payload", "error", err)
		return nil, err
	}

	client := &http.Client{
		Timeout: 300 * time.Second, // セマフォ待ち(160s) + 生成(60s) + マージン
	}

	req, err := http.NewRequestWithContext(ctx, "POST", qualityCheckerAPIURL, strings.NewReader(string(jsonPayload)))
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to create HTTP request", "error", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	response, err := client.Do(req)
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
		logger.Logger.ErrorContext(ctx, "Failed to parse score, attempting fallback", "error", err, "response", responseText)

		// Try emergency fallback parsing strategies
		fallbackScore := attemptEmergencyParsing(responseText)
		if fallbackScore != nil {
			logger.Logger.InfoContext(ctx, "Successfully parsed score using emergency fallback", "score", fallbackScore)
			return fallbackScore, nil
		}

		// Final fallback: if the model is consistently failing, assign low score
		finalFallbackScore := &Score{Overall: 1}
		logger.Logger.WarnContext(ctx, "Using final fallback score due to parsing failure", "score", finalFallbackScore)

		return finalFallbackScore, nil
	}

	logger.Logger.InfoContext(ctx, "Successfully parsed score", "score", score)
	return &score, nil
}

func parseScore(response string) (Score, error) {
	response = strings.TrimSpace(response)
	logger.Logger.Info("Parsing response", "original_response", response)

	// Try to extract score from <score>X</score> pattern
	re := regexp.MustCompile(`<score>(\d+)</score>`)
	matches := re.FindStringSubmatch(response)

	if len(matches) == 2 {
		scoreStr := matches[1]
		score, err := strconv.ParseInt(scoreStr, 10, strconv.IntSize)
		if err != nil {
			logger.Logger.Error("Failed to convert score to integer", "score_str", scoreStr, "error", err)
			return Score{}, fmt.Errorf("failed to convert score to integer: %w", err)
		}

		// Clamp score to valid range (0-30)
		if score < 0 {
			score = 0
		} else if score > 30 {
			score = 30
		}

		return Score{Overall: int(score)}, nil
	}

	// Fallback: try to find any integer in the response
	re = regexp.MustCompile(`\b(\d+)\b`)
	matches = re.FindStringSubmatch(response)

	if len(matches) == 2 {
		scoreStr := matches[1]
		score, err := strconv.ParseInt(scoreStr, 10, strconv.IntSize)
		if err != nil {
			logger.Logger.Error("Failed to convert fallback score to integer", "score_str", scoreStr, "error", err)
			return Score{}, fmt.Errorf("failed to convert fallback score to integer: %w", err)
		}

		// Clamp score to valid range (0-30)
		if score < 0 {
			score = 0
		} else if score > 30 {
			score = 30
		}

		logger.Logger.Info("Used fallback score parsing", "score", score)
		return Score{Overall: int(score)}, nil
	}

	logger.Logger.Error("Could not extract score from response", "response", response)
	return Score{}, fmt.Errorf("could not extract score from response: %s", response)
}

// attemptEmergencyParsing tries very aggressive parsing strategies as a last resort
func attemptEmergencyParsing(response string) *Score {
	// Remove all non-alphanumeric characters except spaces
	cleaned := regexp.MustCompile(`[^\w\s]`).ReplaceAllString(response, " ")

	// Find all integers in the response
	re := regexp.MustCompile(`\b(\d+)\b`)
	numbers := re.FindAllString(cleaned, -1)

	// If we have at least 1 number, use the first one
	if len(numbers) >= 1 {
		score, err := strconv.ParseInt(numbers[0], 10, strconv.IntSize)
		if err == nil {
			// Clamp score to valid range (0-30)
			if score < 0 {
				score = 0
			} else if score > 30 {
				score = 30
			}

			logger.Logger.Info("Emergency parsing successful", "number", numbers[0], "score", score)
			return &Score{Overall: int(score)}
		}
	}

	logger.Logger.Warn("All emergency parsing strategies failed", "response", response)
	return nil
}

// RemoveLowScoreSummary deletes a low-quality summary via the repository and triggers re-fetch of the article.
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

	// Re-fetch article from web after deletion
	if articleRepo == nil {
		return nil
	}

	article, fetchErr := articleRepo.FindByID(ctx, articleWithSummary.ArticleID)
	if fetchErr != nil {
		logger.Logger.WarnContext(ctx, "Failed to get article for re-fetch after summary deletion",
			"articleID", articleWithSummary.ArticleID,
			"error", fetchErr)
	} else if article == nil {
		logger.Logger.WarnContext(ctx, "Article not found for re-fetch after summary deletion",
			"articleID", articleWithSummary.ArticleID)
	} else if article.URL == "" {
		logger.Logger.WarnContext(ctx, "Article URL is empty, cannot re-fetch from web",
			"articleID", articleWithSummary.ArticleID)
	} else {
		client := &http.Client{
			Timeout: 30 * time.Second,
		}
		req, err := http.NewRequestWithContext(ctx, "GET", article.URL, nil)
		if err != nil {
			logger.Logger.WarnContext(ctx, "Failed to create HTTP request for article re-fetch",
				"articleID", articleWithSummary.ArticleID,
				"url", article.URL,
				"error", err)
		} else {
			req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AltBot/1.0; +https://alt.example.com/bot)")
			resp, err := client.Do(req)
			if err != nil {
				logger.Logger.WarnContext(ctx, "Failed to re-fetch article from web after summary deletion",
					"articleID", articleWithSummary.ArticleID,
					"url", article.URL,
					"error", err)
			} else {
				defer func() {
					if closeErr := resp.Body.Close(); closeErr != nil {
						logger.Logger.ErrorContext(ctx, "Failed to close response body", "error", closeErr)
					}
				}()
				logger.Logger.InfoContext(ctx, "Successfully re-fetched article from web after summary deletion",
					"articleID", articleWithSummary.ArticleID,
					"url", article.URL,
					"status_code", resp.StatusCode)
			}
		}
	}

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
func scoreSummaryWithRetry(ctx context.Context, prompt string, maxRetries int) (*Score, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		score, err := scoreSummary(ctx, prompt)
		if err == nil && score != nil {
			return score, nil
		}
		lastErr = err
		logger.Logger.WarnContext(ctx, "Failed to score summary, retrying...", "attempt", attempt+1, "max_retries", maxRetries, "error", err)
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond) // Simple exponential backoff
	}
	return nil, lastErr
}

// JudgeArticleQuality judges the quality of an article's summary and takes action if the score is low.
func JudgeArticleQuality(ctx context.Context, summaryRepo repository.SummaryRepository, articleRepo repository.ArticleRepository, articleWithSummary *driver.ArticleWithSummary) error {
	if articleWithSummary == nil || articleWithSummary.ArticleID == "" {
		return errors.New("article with summary is invalid")
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

	prompt := fmt.Sprintf(JudgeTemplate, articleWithSummary.Content, articleWithSummary.SummaryJapanese)
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
