package qualitychecker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"pre-processor/logger"
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

Return below format:
<score><coherence>?;<relevancy>?;<fluency>?;<overall>?;</score>
Example format:
<score><coherence>5;<relevancy>5;<fluency>5;<overall>5;</score>

<|user|>
### BASE ARTICLE
%s
### JAPANESE SUMMARY
%s
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

func parseScore(score string) (Score, error) {
	score = strings.TrimPrefix(score, "<score>")
	score = strings.TrimSuffix(score, "</score>")
	score = strings.TrimSpace(score)

	coherence := strings.Split(score, ";")[0]
	relevancy := strings.Split(score, ";")[1]
	fluency := strings.Split(score, ";")[2]
	overall := strings.Split(score, ";")[3]
	coherenceFloat, err := strconv.ParseFloat(coherence, 64)
	if err != nil {
		return Score{}, err
	}
	relevancyFloat, err := strconv.ParseFloat(relevancy, 64)
	if err != nil {
		return Score{}, err
	}
	fluencyFloat, err := strconv.ParseFloat(fluency, 64)
	if err != nil {
		return Score{}, err
	}
	overallFloat, err := strconv.ParseFloat(overall, 64)
	if err != nil {
		return Score{}, err
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
