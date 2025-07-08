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
**FORBIDDEN: Any text except the score pattern**
**REQUIRED THIS SCORE PATTERN: <score>coherence:X;relevancy:X;fluency:X;overall:X</score>**
**X is a number between 1 and 10**

**DO NOT write "Okay" or any explanation.**
**ONLY output the score pattern.**

Article: %s
Summary: %s

OUTPUT ONLY THIS:
<score>coherence:X;relevancy:X;fluency:X;overall:X</score>
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

		// Try emergency fallback parsing strategies
		fallbackScore := attemptEmergencyParsing(responseText)
		if fallbackScore != nil {
			logger.Logger.Info("Successfully parsed scores using emergency fallback", "scores", fallbackScore)
			return fallbackScore, nil
		}

		// Final fallback: if the model is consistently failing, assign low scores
		finalFallbackScore := &Score{
			Coherence: 1.0,
			Relevan:   1.0,
			Fluency:   1.0,
			Overall:   1.0,
		}
		logger.Logger.Warn("Using final fallback scores due to parsing failure", "scores", finalFallbackScore)

		return finalFallbackScore, nil
	}

	// Try to parse the extracted JSON
	logger.Logger.Info("Successfully parsed scores", "scores", scores)

	return &scores, nil
}

func parseScore(response string) (Score, error) {
	response = strings.TrimSpace(response)
	logger.Logger.Info("Parsing response", "original_response", response)

	// Try extraction strategies in order of preference
	extractors := []struct {
		name string
		fn   func(string) (Score, bool)
	}{
		{"score_tags", extractFromScoreTags},
		{"named_format", extractFromNamedFormat},
		{"json_format", extractFromJSONFormat},
		{"number_sequence", extractFromNumberSequence},
		{"line_by_line", extractFromLineByLine},
		{"bracketed_numbers", extractFromBracketedNumbers},
		{"flexible_delimiters", extractFromFlexibleDelimiters},
		{"any_numbers", extractFromAnyNumbers},
	}

	for _, extractor := range extractors {
		if score, ok := extractor.fn(response); ok {
			logger.Logger.Info("Successfully extracted scores", "strategy", extractor.name, "scores", score)
			return score, nil
		}
		logger.Logger.Debug("Failed to extract scores", "strategy", extractor.name, "response", response)
	}

	logger.Logger.Error("Could not extract scores from response", "response", response)
	return Score{}, fmt.Errorf("could not extract scores from response: %s", response)
}

// attemptEmergencyParsing tries very aggressive parsing strategies as a last resort
func attemptEmergencyParsing(response string) *Score {
	// Remove all non-alphanumeric characters except dots and spaces
	cleaned := regexp.MustCompile(`[^\w\s\.]`).ReplaceAllString(response, " ")

	// Find all decimal numbers in the response
	re := regexp.MustCompile(`\b(\d+(?:\.\d+)?)\b`)
	numbers := re.FindAllString(cleaned, -1)

	// If we have at least 4 numbers, use the first 4
	if len(numbers) >= 4 {
		if score, ok := buildScore(numbers[0], numbers[1], numbers[2], numbers[3]); ok {
			logger.Logger.Info("Emergency parsing successful", "numbers", numbers[:4], "scores", score)
			return &score
		}
	}

	// If we have at least 1 number, use it for all scores
	if len(numbers) >= 1 {
		score := numbers[0]
		if builtScore, ok := buildScore(score, score, score, score); ok {
			logger.Logger.Info("Emergency parsing with single score", "score", score, "scores", builtScore)
			return &builtScore
		}
	}

	// Try to find any single digit that could be a score
	singleDigitRe := regexp.MustCompile(`\b([1-9])\b`)
	singleDigits := singleDigitRe.FindAllString(response, -1)
	if len(singleDigits) >= 1 {
		score := singleDigits[0]
		if builtScore, ok := buildScore(score, score, score, score); ok {
			logger.Logger.Info("Emergency parsing with single digit", "digit", score, "scores", builtScore)
			return &builtScore
		}
	}

	logger.Logger.Warn("All emergency parsing strategies failed", "response", response)
	return nil
}

// extractFromScoreTags tries to extract scores from <score>...</score> tags.
func extractFromScoreTags(response string) (Score, bool) {
	patterns := []string{
		`<score>(.*?)</score>`,        // With closing tag
		`<score>([^<\n]+)`,            // Without closing tag
		`\*\*score\*\*[:\s]*([^\n]+)`, // **score**: format
		`score\s*:\s*([^\n]+)`,        // score: format
		`\[score\]([^\n]+)`,           // [score] format
		`(?i)score\s*=\s*([^\n]+)`,    // score = format
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?i)` + pattern)
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
	patterns := []string{
		`coherence\s*[:=]\s*(\d+(?:\.\d+)?)[;\s,]*relevancy\s*[:=]\s*(\d+(?:\.\d+)?)[;\s,]*fluency\s*[:=]\s*(\d+(?:\.\d+)?)[;\s,]*overall\s*[:=]\s*(\d+(?:\.\d+)?)`,
		`coherence\s*[:=]\s*(\d+(?:\.\d+)?)[;\s,|/\-_]*relevancy\s*[:=]\s*(\d+(?:\.\d+)?)[;\s,|/\-_]*fluency\s*[:=]\s*(\d+(?:\.\d+)?)[;\s,|/\-_]*overall\s*[:=]\s*(\d+(?:\.\d+)?)`,
		`coherence[:\s]*(\d+(?:\.\d+)?)[^0-9]*relevancy[:\s]*(\d+(?:\.\d+)?)[^0-9]*fluency[:\s]*(\d+(?:\.\d+)?)[^0-9]*overall[:\s]*(\d+(?:\.\d+)?)`,
		`(\d+(?:\.\d+)?)[^\d]*coherence[^0-9]*(\d+(?:\.\d+)?)[^\d]*relevancy[^0-9]*(\d+(?:\.\d+)?)[^\d]*fluency[^0-9]*(\d+(?:\.\d+)?)[^\d]*overall`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?i)` + pattern) // Case insensitive
		if matches := re.FindStringSubmatch(response); len(matches) == 5 {
			return buildScore(matches[1], matches[2], matches[3], matches[4])
		}
	}

	// Try alternative approach - find all named scores separately
	return extractNamedScoresFlexible(response)
}

// extractNamedScoresFlexible tries to find each named score separately with flexible matching
func extractNamedScoresFlexible(response string) (Score, bool) {
	scoreMap := make(map[string]string)

	// Define patterns for each score type with very flexible matching
	scorePatterns := map[string][]string{
		"coherence": {
			`(?i)coherence\s*[:=]\s*(\d+(?:\.\d+)?)`,
			`(?i)coherence\s*[:=]?\s*(\d+(?:\.\d+)?)`,
			`(?i)coherence[^\d]*(\d+(?:\.\d+)?)`,
		},
		"relevancy": {
			`(?i)relevancy\s*[:=]\s*(\d+(?:\.\d+)?)`,
			`(?i)relevancy\s*[:=]?\s*(\d+(?:\.\d+)?)`,
			`(?i)relevancy[^\d]*(\d+(?:\.\d+)?)`,
			`(?i)relevance\s*[:=]\s*(\d+(?:\.\d+)?)`,
		},
		"fluency": {
			`(?i)fluency\s*[:=]\s*(\d+(?:\.\d+)?)`,
			`(?i)fluency\s*[:=]?\s*(\d+(?:\.\d+)?)`,
			`(?i)fluency[^\d]*(\d+(?:\.\d+)?)`,
		},
		"overall": {
			`(?i)overall\s*[:=]\s*(\d+(?:\.\d+)?)`,
			`(?i)overall\s*[:=]?\s*(\d+(?:\.\d+)?)`,
			`(?i)overall[^\d]*(\d+(?:\.\d+)?)`,
			`(?i)total\s*[:=]\s*(\d+(?:\.\d+)?)`,
		},
	}

	// Try to find each score
	for scoreName, patterns := range scorePatterns {
		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(response); len(matches) > 1 {
				key := scoreName
				if key == "relevancy" {
					key = "relevan" // Match struct field name
				}
				scoreMap[key] = matches[1]
				break
			}
		}
	}

	// Check if we have all required scores
	if coherence, ok1 := scoreMap["coherence"]; ok1 {
		if relevan, ok2 := scoreMap["relevan"]; ok2 {
			if fluency, ok3 := scoreMap["fluency"]; ok3 {
				if overall, ok4 := scoreMap["overall"]; ok4 {
					return buildScore(coherence, relevan, fluency, overall)
				}
			}
		}
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

// extractFromJSONFormat tries to extract scores from JSON format
func extractFromJSONFormat(response string) (Score, bool) {
	// Look for JSON-like patterns
	patterns := []string{
		`\{[^}]*"coherence"\s*:\s*(\d+(?:\.\d+)?)[^}]*"relevancy"\s*:\s*(\d+(?:\.\d+)?)[^}]*"fluency"\s*:\s*(\d+(?:\.\d+)?)[^}]*"overall"\s*:\s*(\d+(?:\.\d+)?)[^}]*\}`,
		`\{[^}]*coherence[^:]*:\s*(\d+(?:\.\d+)?)[^}]*relevancy[^:]*:\s*(\d+(?:\.\d+)?)[^}]*fluency[^:]*:\s*(\d+(?:\.\d+)?)[^}]*overall[^:]*:\s*(\d+(?:\.\d+)?)[^}]*\}`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		if matches := re.FindStringSubmatch(response); len(matches) == 5 {
			return buildScore(matches[1], matches[2], matches[3], matches[4])
		}
	}

	return Score{}, false
}

// extractFromLineByLine tries to extract scores from line-by-line format
func extractFromLineByLine(response string) (Score, bool) {
	lines := strings.Split(response, "\n")
	scoreMap := make(map[string]string)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to match patterns like "Coherence: 5" or "- Coherence: 5"
		patterns := []string{
			`(?i)^\s*-?\s*coherence\s*[:=]\s*(\d+(?:\.\d+)?)`,
			`(?i)^\s*-?\s*relevancy\s*[:=]\s*(\d+(?:\.\d+)?)`,
			`(?i)^\s*-?\s*fluency\s*[:=]\s*(\d+(?:\.\d+)?)`,
			`(?i)^\s*-?\s*overall\s*[:=]\s*(\d+(?:\.\d+)?)`,
		}

		keys := []string{"coherence", "relevancy", "fluency", "overall"}

		for i, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(line); len(matches) > 1 {
				key := keys[i]
				if key == "relevancy" {
					key = "relevan" // Match struct field name
				}
				scoreMap[key] = matches[1]
			}
		}
	}

	// Check if we have all required scores
	if coherence, ok1 := scoreMap["coherence"]; ok1 {
		if relevan, ok2 := scoreMap["relevan"]; ok2 {
			if fluency, ok3 := scoreMap["fluency"]; ok3 {
				if overall, ok4 := scoreMap["overall"]; ok4 {
					return buildScore(coherence, relevan, fluency, overall)
				}
			}
		}
	}

	return Score{}, false
}

// extractFromBracketedNumbers tries to extract numbers from various bracket formats
func extractFromBracketedNumbers(response string) (Score, bool) {
	// Look for numbers in brackets, parentheses, or other containers
	patterns := []string{
		`\[(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\]`,
		`\((\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\)`,
		`\{(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\}`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(response); len(matches) == 5 {
			return buildScore(matches[1], matches[2], matches[3], matches[4])
		}
	}

	return Score{}, false
}

// extractFromFlexibleDelimiters tries to extract with very flexible delimiter handling
func extractFromFlexibleDelimiters(response string) (Score, bool) {
	// Remove common noise words and characters
	cleaned := regexp.MustCompile(`(?i)(score|scores|result|results|analysis|evaluation)[:=]?\s*`).ReplaceAllString(response, "")

	// Look for any 4 numbers with very flexible delimiters
	patterns := []string{
		`(\d+(?:\.\d+)?)\s*[;\s,|/\-_]+\s*(\d+(?:\.\d+)?)\s*[;\s,|/\-_]+\s*(\d+(?:\.\d+)?)\s*[;\s,|/\-_]+\s*(\d+(?:\.\d+)?)`,
		`(\d+(?:\.\d+)?)\s+(\d+(?:\.\d+)?)\s+(\d+(?:\.\d+)?)\s+(\d+(?:\.\d+)?)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(cleaned); len(matches) == 5 {
			return buildScore(matches[1], matches[2], matches[3], matches[4])
		}
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
