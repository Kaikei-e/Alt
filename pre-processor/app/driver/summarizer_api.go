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

type OllamaResponse struct {
	Model      string `json:"model"`
	Response   string `json:"response"`
	Done       bool   `json:"done"`
	DoneReason string `json:"done_reason"`
}

const SUMMARIZER_API_URL = "http://news-creator:11434/api/generate"
const MODEL = "phi4-mini:3.8b"

// Refined prompt template optimized for Phi-4-mini-instruct
const PROMPT_TEMPLATE = `<|system|>
You are an expert multilingual journalist and translator specializing in creating high-quality Japanese summaries of English news articles. You have extensive knowledge of both languages and understand cultural nuances.

<|user|>
Please analyze the following English news article and create a comprehensive Japanese summary following these specific requirements:

**TASK**: Create a Japanese summary of the English article below
**LANGUAGE**: Output in Japanese only
**LENGTH**: Maximum 1500 Japanese characters
**STYLE**: Professional news summary style
**FOCUS**: Capture key facts, main points, and important context
**FORMAT**: Single paragraph, no bullet points

**ARTICLE TO SUMMARIZE**:
---
%s
---

**INSTRUCTIONS**:
1. Read and understand the entire article carefully
2. Identify the main facts, key players, and important context
3. Create a flowing, natural Japanese summary that captures the essence
4. Ensure the summary is informative yet concise
5. Maintain journalistic objectivity
6. Output ONLY the Japanese summary text

<|assistant|>
`

func ArticleSummarizerAPIClient(ctx context.Context, article *models.Article) (*SummarizedContent, error) {
	prompt := fmt.Sprintf(PROMPT_TEMPLATE, article.Content)

	payload := payloadModel{
		Model:  MODEL,
		Prompt: prompt,
		Stream: false,
		Options: optionsModel{
			Temperature:   0.2,
			TopP:          0.9,
			NumPredict:    1500, // Reduced since we want concise summaries
			RepeatPenalty: 1.05,
			Stop:          []string{"<|user|>", "<|system|>"},
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
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		logger.Logger.Error("API returned non-200 status", "status", resp.Status, "code", resp.StatusCode)
		return nil, fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Logger.Error("Failed to read response body", "error", err)
		return nil, err
	}

	logger.Logger.Info("Response received", "status", resp.Status)
	logger.Logger.Debug("Response body", "body", string(body))

	// Parse the Ollama API response
	var apiResponse OllamaResponse
	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		logger.Logger.Error("Failed to unmarshal API response", "error", err)
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	// Check if response is complete
	if !apiResponse.Done {
		logger.Logger.Warn("Received incomplete response from API")
	}

	summarizedContent := &SummarizedContent{
		ArticleID:       article.ID,
		SummaryJapanese: apiResponse.Response,
	}

	logger.Logger.Info("Summary generated successfully",
		"article_id", article.ID,
		"summary_length", len(apiResponse.Response))

	return summarizedContent, nil
}

func cleanSummarizedContent(content string) string {
	// Remove any system tags that might leak through
	content = strings.ReplaceAll(content, "<|system|>", "")
	content = strings.ReplaceAll(content, "<|user|>", "")
	content = strings.ReplaceAll(content, "<|assistant|>", "")

	// Remove potential thinking tags
	if startIdx := strings.Index(content, "<think>"); startIdx != -1 {
		if endIdx := strings.Index(content, "</think>"); endIdx != -1 {
			content = content[:startIdx] + content[endIdx+8:]
		}
	}

	// Basic cleanup: trim whitespace and remove empty lines
	lines := strings.Split(content, "\n")
	var cleanLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" &&
			!strings.HasPrefix(trimmed, "---") &&
			!strings.HasPrefix(trimmed, "**") &&
			!strings.Contains(trimmed, "Summary:") &&
			!strings.Contains(trimmed, "要約:") {
			cleanLines = append(cleanLines, trimmed)
		}
	}

	// Join lines with space and return final cleaned content
	result := strings.Join(cleanLines, " ")
	return strings.TrimSpace(result)
}
