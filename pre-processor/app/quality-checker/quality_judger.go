package qualitychecker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"pre-processor/logger"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const QUALITY_CHECKER_API_URL = "http://news-creator:11434/api/generate"
const MODEL = "phi4-mini:3.8b"
const LOW_SCORE_THRESHOLD = 7.0

type judgePrompt struct {
	Model   string       `json:"model"`
	Prompt  string       `json:"prompt"`
	Stream  bool         `json:"stream"`
	Options optionsModel `json:"options"`
}

type optionsModel struct {
	Temperature float64  `json:"temperature"`
	TopP        float64  `json:"top_p"`
	NumPredict  int      `json:"num_predict"`
	NumCtx      int      `json:"num_ctx"`
	Stop        []string `json:"stop"`
}

type ollamaResponse struct {
	Model      string `json:"model"`
	Response   string `json:"response"`
	Done       bool   `json:"done"`
	DoneReason string `json:"done_reason"`
}

type Score struct {
	Coherence float64 `json:"coherence"`
	Relevan   float64 `json:"relevancy"`
	Fluency   float64 `json:"fluency"`
	Overall   float64 `json:"overall"`
}

const JudgeTemplate = `
<|system|>You are a meticulous Japanese news editor.
Rate the following summary from 1 (poor) to 10 (excellent) on:
coherence, relevancy, fluency, overall.

CRITICAL SCORING RULES:
- Empty or missing summary = 1 point for all scores
- Very short summary (less than 10 characters) = maximum 3 points overall
- Broken/garbled text = maximum 2 points overall
- Irrelevant content = maximum 3 points overall

CRITICAL: You MUST respond with ONLY the score tags and numbers. No other text.
CRITICAL: Use this EXACT format with semicolons as separators:
<score><coherence>X;<relevancy>Y;<fluency>Z;<overall>W;</score>

Where X, Y, Z, W are numbers from 1 to 10.

Example format:
<score><coherence>5;<relevancy>5;<fluency>5;<overall>5;</score>

<|user|>
### BASE ARTICLE
%s

### JAPANESE SUMMARY
%s

Rate the Japanese summary above based on how well it summarizes the base article.
If the summary is empty, missing, or contains no meaningful content, give it 1 point for all scores.
Return ONLY the score format shown above.
<|assistant|>`

func scoreSummary(prompt string) (*Score, error) {
	optionsModel := optionsModel{
		Temperature: 0.0,
		TopP:        0.9,
		NumPredict:  120,
		NumCtx:      4096,
		Stop:        []string{"<|end|>"},
	}

	payload := judgePrompt{
		Model:   "phi4-mini:3.8b",
		Prompt:  prompt,
		Stream:  false,
		Options: optionsModel,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		logger.Logger.Error("Failed to marshal payload", "error", err)
		return nil, err
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", QUALITY_CHECKER_API_URL, strings.NewReader(string(jsonPayload)))
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
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		logger.Logger.Error("Failed to read response body", "error", err)
		return nil, err
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		logger.Logger.Error("Failed to unmarshal response", "error", err, "body", string(body))
		return nil, err
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
		logger.Logger.Error("Failed to parse score", "error", err, "response", responseText)
		return nil, err
	}

	// Try to parse the extracted JSON
	logger.Logger.Info("Successfully parsed scores", "scores", scores)
	return &scores, nil
}

func parseScore(response string) (Score, error) {
	response = strings.TrimSpace(response)

	// Try to extract score content between <score> tags using regex
	re := regexp.MustCompile(`<score>(.*?)</score>`)
	matches := re.FindStringSubmatch(response)

	var scoreContent string
	if len(matches) > 1 {
		scoreContent = strings.TrimSpace(matches[1])
	} else {
		logger.Logger.Error("Could not find score content in response", "response", response)
		// Fallback: try to extract without tags
		scoreContent = strings.TrimPrefix(response, "<score>")
		scoreContent = strings.TrimSuffix(scoreContent, "</score>")
		scoreContent = strings.TrimSpace(scoreContent)

		// If still no valid content, return error
		if scoreContent == "" || scoreContent == response {
			logger.Logger.Error("Could not find score content in response", "response", response)
			return Score{}, errors.New("could not find score content in response: " + response)
		}
	}

	// Split the score and check if we have exactly 4 parts
	parts := strings.Split(scoreContent, ";")

	// Filter out empty parts (handles trailing semicolons)
	var validParts []string
	for _, part := range parts {
		trimmedPart := strings.TrimSpace(part)
		if trimmedPart != "" {
			validParts = append(validParts, trimmedPart)
		}
	}

	if len(validParts) != 4 {
		logger.Logger.Error("Expected 4 score parts separated by semicolons, got", "parts", len(validParts), "scoreContent", scoreContent, "validParts", validParts)
		return Score{}, errors.New("expected 4 score parts separated by semicolons, got " + strconv.Itoa(len(validParts)) + " parts: " + scoreContent)
	}

	// Clean each part and remove any potential XML-like tags
	coherence := strings.TrimSpace(strings.ReplaceAll(validParts[0], "<coherence>", ""))
	relevancy := strings.TrimSpace(strings.ReplaceAll(validParts[1], "<relevancy>", ""))
	fluency := strings.TrimSpace(strings.ReplaceAll(validParts[2], "<fluency>", ""))
	overall := strings.TrimSpace(strings.ReplaceAll(validParts[3], "<overall>", ""))

	coherenceFloat, err := strconv.ParseFloat(coherence, 64)
	if err != nil {
		logger.Logger.Error("Failed to parse coherence score", "error", err, "coherence", coherence)
		return Score{}, errors.New("failed to parse coherence score: " + coherence)
	}
	relevancyFloat, err := strconv.ParseFloat(relevancy, 64)
	if err != nil {
		logger.Logger.Error("Failed to parse relevancy score", "error", err, "relevancy", relevancy)
		return Score{}, errors.New("failed to parse relevancy score: " + relevancy)
	}
	fluencyFloat, err := strconv.ParseFloat(fluency, 64)
	if err != nil {
		logger.Logger.Error("Failed to parse fluency score", "error", err, "fluency", fluency)
		return Score{}, errors.New("failed to parse fluency score: " + fluency)
	}
	overallFloat, err := strconv.ParseFloat(overall, 64)
	if err != nil {
		logger.Logger.Error("Failed to parse overall score", "error", err, "overall", overall)
		return Score{}, errors.New("failed to parse overall score: " + overall)
	}

	// Validate scores are within expected range
	if coherenceFloat < 1 || coherenceFloat > 10 {
		logger.Logger.Error("Coherence score is out of range", "score", coherenceFloat)
		return Score{}, errors.New("coherence score " + strconv.FormatFloat(coherenceFloat, 'f', -1, 64) + " is out of range (1-10)")
	}
	if relevancyFloat < 1 || relevancyFloat > 10 {
		logger.Logger.Error("Relevancy score is out of range", "score", relevancyFloat)
		return Score{}, errors.New("relevancy score " + strconv.FormatFloat(relevancyFloat, 'f', -1, 64) + " is out of range (1-10)")
	}
	if fluencyFloat < 1 || fluencyFloat > 10 {
		logger.Logger.Error("Fluency score is out of range", "score", fluencyFloat)
		return Score{}, errors.New("fluency score " + strconv.FormatFloat(fluencyFloat, 'f', -1, 64) + " is out of range (1-10)")
	}
	if overallFloat < 1 || overallFloat > 10 {
		logger.Logger.Error("Overall score is out of range", "score", overallFloat)
		return Score{}, errors.New("overall score " + strconv.FormatFloat(overallFloat, 'f', -1, 64) + " is out of range (1-10)")
	}

	parsedScore := Score{
		Coherence: coherenceFloat,
		Relevan:   relevancyFloat,
		Fluency:   fluencyFloat,
		Overall:   overallFloat,
	}

	return parsedScore, nil
}

func RemoveLowScoreSummary(ctx context.Context, dbPool *pgxpool.Pool, articleWithScore *ArticleWithScore) error {
	// Create the proper prompt for scoring
	prompt := fmt.Sprintf(JudgeTemplate, articleWithScore.Content, articleWithScore.SummaryJapanese)

	scores, err := scoreSummary(prompt)
	if err != nil {
		logger.Logger.Error("Failed to score summary", "error", err, "articleID", articleWithScore.ArticleID)
		return err
	}

	if scores == nil {
		logger.Logger.Error("Received nil scores", "articleID", articleWithScore.ArticleID)
		return errors.New("received nil scores for article " + articleWithScore.ArticleID)
	}

	logger.Logger.Info("Article quality scores",
		"articleID", articleWithScore.ArticleID,
		"coherence", scores.Coherence,
		"relevancy", scores.Relevan,
		"fluency", scores.Fluency,
		"overall", scores.Overall)

	// If score is too low, remove the summary (but keep the article)
	if scores.Overall < LOW_SCORE_THRESHOLD {
		logger.Logger.Info("Removing low quality summary",
			"articleID", articleWithScore.ArticleID,
			"score", scores.Overall,
			"threshold", LOW_SCORE_THRESHOLD)

		txOptions := pgx.TxOptions{
			IsoLevel: pgx.RepeatableRead,
		}

		tx, err := dbPool.BeginTx(ctx, txOptions)
		if err != nil {
			logger.Logger.Error("Failed to begin transaction", "error", err)
			return errors.New("failed to begin transaction")
		}

		_, err = tx.Exec(ctx, "DELETE FROM article_summaries WHERE article_id = $1", articleWithScore.ArticleID)
		if err != nil {
			tx.Rollback(ctx)
			logger.Logger.Error("Failed to delete article summary", "error", err, "articleID", articleWithScore.ArticleID)
			return errors.New("failed to delete article summary")
		}

		err = tx.Commit(ctx)
		if err != nil {
			logger.Logger.Error("Failed to commit transaction", "error", err)
			return errors.New("failed to commit transaction")
		}

		logger.Logger.Info("Deleted low quality article summary", "articleID", articleWithScore.ArticleID)
	} else {
		logger.Logger.Info("Summary quality is acceptable", "articleID", articleWithScore.ArticleID, "score", scores.Overall)
	}

	return nil
}
