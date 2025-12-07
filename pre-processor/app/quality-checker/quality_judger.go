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

	logger "pre-processor/utils/logger"

	"pre-processor/driver"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// qualityCheckerAPIURL can be overridden in tests
	qualityCheckerAPIURL = "http://news-creator:11434/api/generate"
	modelName            = "gemma3:4b"
	lowScoreThreshold    = 7 // 7 is the lowest score that is acceptable
)

type judgePrompt struct {
	Model   string       `json:"model"`
	Prompt  string       `json:"prompt"`
	Options optionsModel `json:"options"`
	Stream  bool         `json:"stream"`
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
		NumCtx:      2048, // Smaller context to focus on the task
		Stop:        []string{"</score>", "\n\n", "ARTICLE:", "SUMMARY:", "<|user|>", "<|assistant|>"},
	}

	payload := judgePrompt{
		Model:   modelName,
		Prompt:  prompt,
		Stream:  false,
		Options: opts,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		logger.Logger.Error("Failed to marshal payload", "error", err)
		return nil, err
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", qualityCheckerAPIURL, strings.NewReader(string(jsonPayload)))
	if err != nil {
		logger.Logger.Error("Failed to create HTTP request", "error", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	response, err := client.Do(req)
	if err != nil {
		logger.Logger.Error("Failed to send HTTP request", "error", err)
		return nil, err
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			logger.Logger.Error("failed to close response body", "error", err)
		}
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		logger.Logger.Error("Failed to read response body", "error", err)
		return nil, err
	}

	var ollamaResp ollamaResponse
	if err = json.Unmarshal(body, &ollamaResp); err != nil {
		logger.Logger.Error("Failed to unmarshal response", "error", err, "body", string(body))
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	logger.Logger.Info("Received ollama response", "done", ollamaResp.Done, "response", ollamaResp.Response)

	if !ollamaResp.Done {
		logger.Logger.Error("Ollama response not completed", "response", ollamaResp.Response)
		return nil, errors.New("ollama response not completed")
	}

	// Extract score from the response
	responseText := strings.TrimSpace(ollamaResp.Response)

	score, err := parseScore(responseText)
	if err != nil {
		logger.Logger.Error("Failed to parse score, attempting fallback", "error", err, "response", responseText)

		// Try emergency fallback parsing strategies
		fallbackScore := attemptEmergencyParsing(responseText)
		if fallbackScore != nil {
			logger.Logger.Info("Successfully parsed score using emergency fallback", "score", fallbackScore)
			return fallbackScore, nil
		}

		// Final fallback: if the model is consistently failing, assign low score
		finalFallbackScore := &Score{Overall: 1}
		logger.Logger.Warn("Using final fallback score due to parsing failure", "score", finalFallbackScore)

		return finalFallbackScore, nil
	}

	logger.Logger.Info("Successfully parsed score", "score", score)
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

func RemoveLowScoreSummary(ctx context.Context, dbPool *pgxpool.Pool, articleWithSummary *driver.ArticleWithSummary) error {
	// Validate dbPool is not nil
	if dbPool == nil {
		return errors.New("database pool is nil, cannot delete summary")
	}

	// Create the proper prompt for scoring
	prompt := fmt.Sprintf(JudgeTemplate, articleWithSummary.Content, articleWithSummary.SummaryJapanese)

	score, err := scoreSummaryWithRetry(ctx, prompt, 3)
	if err != nil {
		// Check if this is a connection error (service unavailable)
		if isConnectionError(err) {
			logger.Logger.Warn("Connection error while scoring summary in RemoveLowScoreSummary, aborting deletion to prevent data loss",
				"error", err,
				"articleID", articleWithSummary.ArticleID)
			// Return error without deleting data - this indicates service unavailability, not low quality
			return fmt.Errorf("failed to connect to news-creator service: %w", err)
		}

		// For non-connection errors, we should not delete data
		logger.Logger.Error("Failed to score summary after retries (non-connection error) in RemoveLowScoreSummary",
			"error", err,
			"articleID", articleWithSummary.ArticleID)
		return fmt.Errorf("failed to score summary (non-connection error): %w", err)
	}

	if score == nil {
		logger.Logger.Error("Received nil score", "articleID", articleWithSummary.ArticleID)
		return errors.New("received nil score for article " + articleWithSummary.ArticleID)
	}

	logger.Logger.Info("Article quality score",
		"articleID", articleWithSummary.ArticleID,
		"score", score.Overall)

	// If score is too low, remove the summary (but keep the article)
	// This only happens when we successfully got a score from the service
	if score.Overall < lowScoreThreshold {
		logger.Logger.Info("Removing low quality summary",
			"articleID", articleWithSummary.ArticleID,
			"score", score.Overall,
			"threshold", lowScoreThreshold)

		txOptions := pgx.TxOptions{
			IsoLevel: pgx.RepeatableRead,
		}

		tx, err := dbPool.BeginTx(ctx, txOptions)
		if err != nil {
			logger.Logger.Error("Failed to begin transaction", "error", err)
			return errors.New("failed to begin transaction")
		}

		_, err = tx.Exec(ctx, "DELETE FROM article_summaries WHERE article_id = $1", articleWithSummary.ArticleID)
		if err != nil {
			err = tx.Rollback(ctx)
			if err != nil {
				logger.Logger.Error("Failed to rollback transaction", "error", err)
			}
			logger.Logger.Error("Failed to delete article summary", "error", err, "articleID", articleWithSummary.ArticleID)

			return errors.New("failed to delete article summary")
		}

		err = tx.Commit(ctx)
		if err != nil {
			logger.Logger.Error("Failed to commit transaction", "error", err)
			return errors.New("failed to commit transaction")
		}

		logger.Logger.Info("Deleted low quality article summary", "articleID", articleWithSummary.ArticleID)
	} else {
		logger.Logger.Info("Summary quality is acceptable", "articleID", articleWithSummary.ArticleID, "score", score.Overall)
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
		logger.Logger.Warn("Failed to score summary, retrying...", "attempt", attempt+1, "max_retries", maxRetries, "error", err)
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond) // Simple exponential backoff
	}
	return nil, lastErr
}

// JudgeArticleQuality judges the quality of an article's summary and takes action if the score is low.
func JudgeArticleQuality(ctx context.Context, dbPool *pgxpool.Pool, articleWithSummary *driver.ArticleWithSummary) error {
	if articleWithSummary == nil || articleWithSummary.ArticleID == "" {
		return errors.New("article with summary is invalid")
	}

	prompt := fmt.Sprintf(JudgeTemplate, articleWithSummary.Content, articleWithSummary.SummaryJapanese)
	score, err := scoreSummaryWithRetry(ctx, prompt, 3)
	if err != nil {
		// Check if this is a connection error (service unavailable)
		if isConnectionError(err) {
			logger.Logger.Warn("Connection error while scoring summary, skipping quality check to prevent data loss",
				"articleID", articleWithSummary.ArticleID,
				"error", err)
			// Return error without deleting data - this indicates service unavailability, not low quality
			return fmt.Errorf("failed to connect to news-creator service: %w", err)
		}

		// For non-connection errors (e.g., parsing errors, invalid responses), we still need to handle them
		// However, we should not delete data based on connection failures
		logger.Logger.Error("Failed to get summary score after retries (non-connection error)",
			"articleID", articleWithSummary.ArticleID,
			"error", err)
		// For non-connection errors, we skip the quality check rather than deleting data
		return fmt.Errorf("failed to score summary (non-connection error): %w", err)
	}

	if score == nil {
		return errors.New("received nil score for article " + articleWithSummary.ArticleID)
	}

	// If score is too low, remove the summary (but keep the article)
	// This only happens when we successfully got a score from the service
	if score.Overall < lowScoreThreshold {
		logger.Logger.Info("Removing low quality summary",
			"articleID", articleWithSummary.ArticleID,
			"score", score.Overall,
			"threshold", lowScoreThreshold)
		return RemoveLowScoreSummary(ctx, dbPool, articleWithSummary)
	}

	logger.Logger.Info("Summary quality is acceptable", "articleID", articleWithSummary.ArticleID, "score", score.Overall)
	return nil
}
