package qualitychecker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const (
	qualityCheckerAPIURL = "http://news-creator:11434/api/generate"
	modelName            = "gemma3:4b"
	lowScoreThreshold    = 7.0
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
	Coherence float64 `json:"coherence"`
	Relevan   float64 `json:"relevancy"`
	Fluency   float64 `json:"fluency"`
	Overall   float64 `json:"overall"`
}

const JudgeTemplate = `
<start_of_turn>user
OUTPUT ONLY THIS EXACT FORMAT - NO OTHER TEXT:
<score>coherence:X;relevancy:Y;fluency:Z;overall:W</score>

Where X,Y,Z,W are single digits 1-10.

Rate this Japanese summary of the English article:

ARTICLE:
%s

SUMMARY:
%s

NOW OUTPUT THE SCORE LINE ONLY.
<end_of_turn>
<start_of_turn>model
`

func scoreSummary(ctx context.Context, prompt string) (*Score, error) {
	// Use more restrictive parameters to encourage format compliance
	opts := optionsModel{
		Temperature: 0.0,  // Very low temperature for more deterministic output
		TopP:        0.5,  // More restrictive sampling
		NumPredict:  80,   // Shorter to force concise responses
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

	client := &http.Client{}

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

	// Extract JSON from the response, handling markdown code blocks
	responseText := strings.TrimSpace(ollamaResp.Response)

	scores, err := parseScore(responseText)
	if err != nil {
		logger.Logger.Error("Failed to parse score, attempting fallback", "error", err, "response", responseText)

		// Fallback: if the model is consistently failing, assign low scores
		fallbackScore := &Score{
			Coherence: 1.0,
			Relevan:   1.0,
			Fluency:   1.0,
			Overall:   1.0,
		}
		logger.Logger.Warn("Using fallback scores due to parsing failure", "scores", fallbackScore)

		return fallbackScore, nil
	}

	// Try to parse the extracted JSON
	logger.Logger.Info("Successfully parsed scores", "scores", scores)

	return &scores, nil
}

func parseScore(response string) (Score, error) {
	response = strings.TrimSpace(response)
	logger.Logger.Info("Parsing response", "original_response", response)

	// Try extraction strategies in order of preference
	extractors := []func(string) (Score, bool){
		extractFromScoreTags,
		extractFromNamedFormat,
		extractFromNumberSequence,
		extractFromAnyNumbers,
	}

	for i, extractor := range extractors {
		if score, ok := extractor(response); ok {
			logger.Logger.Info("Successfully extracted scores", "strategy", i, "scores", score)
			return score, nil
		}
	}

	logger.Logger.Error("Could not extract scores from response", "response", response)

	return Score{}, fmt.Errorf("could not extract scores from response: %s", response)
}

// extractFromScoreTags tries to extract scores from <score>...</score> tags.
func extractFromScoreTags(response string) (Score, bool) {
	patterns := []string{
		`<score>(.*?)</score>`, // With closing tag
		`<score>([^<\n]+)`,     // Without closing tag
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(response); len(matches) > 1 {
			content := strings.TrimSpace(matches[1])
			if score, ok := parseScoreContent(content); ok {
				return score, true
			}
		}
	}

	return Score{}, false
}

// extractFromNamedFormat tries to extract from named format like "coherence:5;relevancy:6...".
func extractFromNamedFormat(response string) (Score, bool) {
	// Look for named scores with flexible separators and spacing
	pattern := `coherence\s*[:=]\s*(\d+(?:\.\d+)?)[;\s,]*relevancy\s*[:=]\s*(\d+(?:\.\d+)?)[;\s,]*fluency\s*[:=]\s*(\d+(?:\.\d+)?)[;\s,]*overall\s*[:=]\s*(\d+(?:\.\d+)?)`
	re := regexp.MustCompile(`(?i)` + pattern) // Case insensitive

	if matches := re.FindStringSubmatch(response); len(matches) == 5 {
		return buildScore(matches[1], matches[2], matches[3], matches[4])
	}

	return Score{}, false
}

// extractFromNumberSequence tries to extract 4 consecutive numbers.
func extractFromNumberSequence(response string) (Score, bool) {
	// Look for exactly 4 numbers separated by common delimiters
	pattern := `(\d+(?:\.\d+)?)[;\s,]+(\d+(?:\.\d+)?)[;\s,]+(\d+(?:\.\d+)?)[;\s,]+(\d+(?:\.\d+)?)`
	re := regexp.MustCompile(pattern)

	if matches := re.FindStringSubmatch(response); len(matches) == 5 {
		return buildScore(matches[1], matches[2], matches[3], matches[4])
	}

	return Score{}, false
}

// extractFromAnyNumbers tries to find any 4 numbers in the response (fallback).
func extractFromAnyNumbers(response string) (Score, bool) {
	re := regexp.MustCompile(`\b(\d+(?:\.\d+)?)\b`)
	numbers := re.FindAllString(response, -1)

	if len(numbers) >= 4 {
		return buildScore(numbers[0], numbers[1], numbers[2], numbers[3])
	}

	return Score{}, false
}

// parseScoreContent parses content from score tags (handles both named and unnamed formats).
func parseScoreContent(content string) (Score, bool) {
	// Try named format first
	if strings.Contains(content, ":") || strings.Contains(content, "=") {
		return parseNamedScores(content)
	}

	// Try simple number sequence
	return parseNumberSequence(content)
}

// parseNamedScores parses "coherence:5;relevancy:6;fluency:4;overall:5" format.
func parseNamedScores(content string) (Score, bool) {
	scoreMap := make(map[string]string)

	// Split by common delimiters
	parts := regexp.MustCompile(`[;,\n]`).Split(content, -1)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Look for key:value or key=value
		var key, value string
		if colonIdx := strings.Index(part, ":"); colonIdx != -1 {
			key = strings.TrimSpace(part[:colonIdx])
			value = strings.TrimSpace(part[colonIdx+1:])
		} else if equalsIdx := strings.Index(part, "="); equalsIdx != -1 {
			key = strings.TrimSpace(part[:equalsIdx])
			value = strings.TrimSpace(part[equalsIdx+1:])
		} else {
			continue
		}

		// Extract number from value
		if numberMatch := regexp.MustCompile(`(\d+(?:\.\d+)?)`).FindString(value); numberMatch != "" {
			// Normalize key name
			key = strings.ToLower(strings.TrimSpace(key))
			if key == "relevancy" {
				key = "relevan" // Match the struct field name
			}

			scoreMap[key] = numberMatch
		}
	}

	// Check if we have all required scores
	coherence, hasCoherence := scoreMap["coherence"]
	relevan, hasRelevan := scoreMap["relevan"]
	fluency, hasFluency := scoreMap["fluency"]
	overall, hasOverall := scoreMap["overall"]

	if hasCoherence && hasRelevan && hasFluency && hasOverall {
		return buildScore(coherence, relevan, fluency, overall)
	}

	return Score{}, false
}

// parseNumberSequence parses simple number sequences like "5;6;4;5" or "5,6,4,5".
func parseNumberSequence(content string) (Score, bool) {
	parts := regexp.MustCompile(`[;,\s]+`).Split(content, -1)
	numbers := make([]string, 0, 4)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if regexp.MustCompile(`^\d+(?:\.\d+)?$`).MatchString(part) {
			numbers = append(numbers, part)
		}
	}

	if len(numbers) >= 4 {
		return buildScore(numbers[0], numbers[1], numbers[2], numbers[3])
	}

	return Score{}, false
}

// buildScore creates a Score struct from string values with validation.
func buildScore(coherence, relevancy, fluency, overall string) (Score, bool) {
	coherenceFloat, err1 := strconv.ParseFloat(coherence, 64)
	relevancyFloat, err2 := strconv.ParseFloat(relevancy, 64)
	fluencyFloat, err3 := strconv.ParseFloat(fluency, 64)
	overallFloat, err4 := strconv.ParseFloat(overall, 64)

	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return Score{}, false
	}

	return Score{
		Coherence: clampScore(coherenceFloat),
		Relevan:   clampScore(relevancyFloat),
		Fluency:   clampScore(fluencyFloat),
		Overall:   clampScore(overallFloat),
	}, true
}

// clampScore ensures the score is within the valid range of 1-10.
func clampScore(score float64) float64 {
	if score < 1.0 {
		return 1.0
	}

	if score > 10.0 {
		return 10.0
	}

	return score
}

// scoreSummaryWithRetry attempts to score a summary with retries and exponential backoff.
func scoreSummaryWithRetry(ctx context.Context, prompt string, maxRetries int) (*Score, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		scores, err := scoreSummary(ctx, prompt)
		if err == nil && scores != nil {
			return scores, nil
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
	scores, err := scoreSummaryWithRetry(ctx, prompt, 3)
	if err != nil {
		logger.Logger.Error("Failed to get summary score after retries", "articleID", articleWithSummary.ArticleID, "error", err)
		// Fallback: if scoring consistently fails, assign a low score to be safe
		scores = &Score{Overall: 1.0}
		logger.Logger.Warn("Using fallback scores due to persistent failures", "articleID", articleWithSummary.ArticleID, "scores", scores)
	}

	if scores == nil {
		return errors.New("received nil scores for article " + articleWithSummary.ArticleID)
	}

	// If score is too low, remove the summary (but keep the article)
	if scores.Overall < lowScoreThreshold {
		logger.Logger.Info("Removing low quality summary",
			"articleID", articleWithSummary.ArticleID,
			"score", scores.Overall,
			"threshold", lowScoreThreshold)
		return RemoveLowScoreSummary(ctx, dbPool, articleWithSummary)
	}

	logger.Logger.Info("Summary quality is acceptable", "articleID", articleWithSummary.ArticleID, "score", scores.Overall)
	return nil
}

func RemoveLowScoreSummary(ctx context.Context, dbPool *pgxpool.Pool, articleWithSummary *driver.ArticleWithSummary) error {
	// Create the proper prompt for scoring
	prompt := fmt.Sprintf(JudgeTemplate, articleWithSummary.Content, articleWithSummary.SummaryJapanese)

	scores, err := scoreSummaryWithRetry(ctx, prompt, 3)
	if err != nil {
		logger.Logger.Error("Failed to score summary after retries", "error", err, "articleID", articleWithSummary.ArticleID)
		// Use fallback scoring instead of failing completely
		scores = &Score{
			Coherence: 1.0,
			Relevan:   1.0,
			Fluency:   1.0,
			Overall:   1.0,
		}
		logger.Logger.Warn("Using fallback scores due to persistent failures", "articleID", articleWithSummary.ArticleID, "scores", scores)
	}

	if scores == nil {
		logger.Logger.Error("Received nil scores", "articleID", articleWithSummary.ArticleID)
		return errors.New("received nil scores for article " + articleWithSummary.ArticleID)
	}

	logger.Logger.Info("Article quality scores",
		"articleID", articleWithSummary.ArticleID,
		"coherence", scores.Coherence,
		"relevancy", scores.Relevan,
		"fluency", scores.Fluency,
		"overall", scores.Overall)

	// If score is too low, remove the summary (but keep the article)
	if scores.Overall < lowScoreThreshold {
		logger.Logger.Info("Removing low quality summary",
			"articleID", articleWithSummary.ArticleID,
			"score", scores.Overall,
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
		logger.Logger.Info("Summary quality is acceptable", "articleID", articleWithSummary.ArticleID, "score", scores.Overall)
	}

	return nil
}
