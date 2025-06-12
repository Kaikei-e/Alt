package articlefetcher

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"pre-processor/logger"
	"pre-processor/models"

	"github.com/go-shiori/go-readability"
)

func FetchArticle(url url.URL) (*models.Article, error) {
	logger.Logger.Info("Fetching article", "url", url.String())

	if urlMP3Validator(url) {
		logger.Logger.Info("Skipping MP3 URL", "url", url.String())
		return nil, nil
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Fetch the page
	resp, err := client.Get(url.String())
	if err != nil {
		logger.Logger.Error("Failed to fetch page", "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Parse the page with readability
	article, err := readability.FromReader(resp.Body, &url)
	if err != nil {
		logger.Logger.Error("Failed to parse article", "error", err)
		return nil, err
	}

	logger.Logger.Info("Article fetched", "title", article.Title, "content length", len(article.TextContent))

	return &models.Article{
		Title:   article.Title,
		Content: article.TextContent,
		URL:     url.String(),
	}, nil
}

func urlMP3Validator(url url.URL) bool {
	return strings.HasSuffix(url.String(), ".mp3")
}
