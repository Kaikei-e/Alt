package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"pre-processor/models"
	"strings"

	"pre-processor/logger"
)

type SummarizedContent struct {
	ArticleID       string `json:"article_id"`
	SummaryJapanese string `json:"summary_japanese"`
}

type payloadModel struct {
	Model   string       `json:"model"`
	Prompt  string       `json:"prompt"`
	Stream  bool         `json:"stream"`
	Options optionsModel `json:"options"`
}

type optionsModel struct {
	Temperature   float64  `json:"temperature"`
	TopP          float64  `json:"top_p"`
	NumPredict    int      `json:"num_predict"`
	RepeatPenalty float64  `json:"repeat_penalty"`
	Stop          []string `json:"stop"`
}

const SUMMARIZER_API_URL = "http://news-creator:11434/api/generate"
const MODEL = "qwen3:4b"
const PROMPT_TEMPLATE_WITH_ARTICLE = `
%s

English Article:
EOP

Japanese Summary:
EOP
`

const PROMPT_TEMPLATE_WITH_RESTRECTION = `
You are a professional Japanese translator and summarizer. Your task is to:
1. Read the English article below
2. Create a concise summary in Japanese
3. Keep it under 1500 Japanese characters
4. Output ONLY the Japanese summary, nothing else

--------------------------------
%s
`

func ArticleSummarizerAPIClient(ctx context.Context, article *models.Article) (*SummarizedContent, error) {
	completePrompt := fmt.Sprintf(PROMPT_TEMPLATE_WITH_ARTICLE, article.Content)
	restrictionPrompt := fmt.Sprintf(PROMPT_TEMPLATE_WITH_RESTRECTION, completePrompt)

	payload := payloadModel{
		Model:  MODEL,
		Prompt: restrictionPrompt,
		Stream: false,
		Options: optionsModel{
			Temperature:   0.3,
			TopP:          0.8,
			NumPredict:    1500,
			RepeatPenalty: 1.1,
			Stop:          []string{"Article:", "English:", "\n\n---", "Summary:", "Note:"},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Logger.Error("Failed to marshal payload", "error", err)
		return nil, err
	}
	client := &http.Client{}

	req, err := http.NewRequest("POST", SUMMARIZER_API_URL, strings.NewReader(string(jsonData)))
	if err != nil {
		logger.Logger.Error("Failed to create request", "error", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.Logger.Error("Failed to send request", "error", err)
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Error("Failed to read response body", "error", err)
		return nil, err
	}

	logger.Logger.Info("Response body", "body is:", string(body))

	// Parse the JSON response to extract the actual response content
	var apiResponse struct {
		Response string `json:"response"`
	}

	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		logger.Logger.Error("Failed to unmarshal API response", "error", err)
		return nil, err
	}

	cleanedContent := CleanSummarizedContent(apiResponse.Response)

	summarizedContent := &SummarizedContent{
		ArticleID:       article.ID,
		SummaryJapanese: cleanedContent,
	}

	return summarizedContent, nil
}

func CleanSummarizedContent(content string) string {
	//  same thing in Go as the following command
	// 	echo "$RESPONSE" | \
	//   sed -n '/<\/think>/,$p' | \
	//   sed '1d' | \
	//   sed '/^$/d' | \
	//   awk 'length > maxlen {maxlen=length; maxline=$0} END {print maxline}' | \
	//   sed 's/^[[:space:]]*//' | \
	//   sed 's/[[:space:]]*$//'

	// First, find the </think> tag and get everything after it
	thinkEndIndex := strings.Index(content, "</think>")
	if thinkEndIndex != -1 {
		// Get everything after </think>
		content = content[thinkEndIndex+8:] // +8 for len("</think>")
	}
	// If no think tag found, use the whole content as is

	// Split into lines and remove empty lines
	lines := strings.Split(content, "\n")
	var nonEmptyLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			nonEmptyLines = append(nonEmptyLines, trimmed)
		}
	}

	// Find the longest line (mimicking the awk command)
	longestLine := ""
	for _, line := range nonEmptyLines {
		if len(line) > len(longestLine) {
			longestLine = line
		}
	}

	// Trim leading and trailing whitespace
	return strings.TrimSpace(longestLine)
}
