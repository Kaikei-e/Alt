package rest_feeds

import (
	"alt/config"
	"alt/di"
	"alt/utils/errors"
	"alt/utils/logger"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
)

func RestHandleFetchInoreaderSummary(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req FeedSummaryRequest
		if err := c.Bind(&req); err != nil {
			return HandleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		// Manual validation - check if feed_urls is provided and within limits
		if len(req.FeedURLs) == 0 {
			return HandleValidationError(c, "feed_urls is required and cannot be empty", "feed_urls", req.FeedURLs)
		}
		if len(req.FeedURLs) > 50 {
			return HandleValidationError(c, "Maximum 50 URLs allowed per request", "feed_urls", len(req.FeedURLs))
		}

		// SSRF protection for all URLs
		for _, feedURL := range req.FeedURLs {
			parsedURL, err := url.Parse(feedURL)
			if err != nil {
				return HandleValidationError(c, "Invalid URL format", "feed_urls", feedURL)
			}

			if err := IsAllowedURL(parsedURL); err != nil {
				securityErr := errors.NewValidationContextError(
					"URL not allowed for security reasons",
					"rest",
					"RESTHandler",
					"fetch_inoreader_summary",
					map[string]interface{}{
						"url":         feedURL,
						"reason":      err.Error(),
						"path":        c.Request().URL.Path,
						"method":      c.Request().Method,
						"remote_addr": c.Request().RemoteAddr,
						"request_id":  c.Response().Header().Get("X-Request-ID"),
					},
				)
				logger.Logger.Error("URL validation failed", "error", securityErr.Error(), "url", feedURL)
				return c.JSON(securityErr.HTTPStatusCode(), securityErr.ToHTTPResponse())
			}
		}

		// Log requested URLs for debugging
		logger.Logger.Info("Fetching summaries for URLs", "urls", req.FeedURLs, "url_count", len(req.FeedURLs))

		// Execute usecase
		summaries, err := container.FetchInoreaderSummaryUsecase.Execute(c.Request().Context(), req.FeedURLs)
		if err != nil {
			return HandleError(c, err, "fetch_inoreader_summary")
		}

		// Convert domain entities to response DTOs
		responses := make([]InoreaderSummaryResponse, 0, len(summaries))
		for _, summary := range summaries {
			authorStr := ""
			if summary.Author != nil {
				authorStr = *summary.Author
			}

			resp := InoreaderSummaryResponse{
				ArticleURL:  summary.ArticleURL,
				Title:       summary.Title,
				Author:      authorStr,
				Content:     summary.Content,
				ContentType: summary.ContentType,
				PublishedAt: summary.PublishedAt.Format(time.RFC3339),
				FetchedAt:   summary.FetchedAt.Format(time.RFC3339),
				InoreaderID: summary.InoreaderID,
			}
			responses = append(responses, resp)
		}

		// Build final response
		finalResponse := FeedSummaryProvidedResponse{
			MatchedArticles: responses,
			TotalMatched:    len(responses),
			RequestedCount:  len(req.FeedURLs),
		}

		// Set caching headers (15 minutes as per XPLAN11.md)
		c.Response().Header().Set("Cache-Control", "public, max-age=900")
		c.Response().Header().Set("Content-Type", "application/json")

		return c.JSON(http.StatusOK, finalResponse)
	}
}

// handleFetchArticleSummary handles article summary fetch requests
// It fetches summaries from alt-db if available, or generates them via news-creator
func RestHandleFetchArticleSummary(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req FeedSummaryRequest
		if err := c.Bind(&req); err != nil {
			return HandleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		// Manual validation - check if feed_urls is provided and within limits
		if len(req.FeedURLs) == 0 {
			return HandleValidationError(c, "feed_urls is required and cannot be empty", "feed_urls", req.FeedURLs)
		}
		if len(req.FeedURLs) > 50 {
			return HandleValidationError(c, "Maximum 50 URLs allowed per request", "feed_urls", len(req.FeedURLs))
		}

		// SSRF protection for all URLs
		for _, feedURL := range req.FeedURLs {
			parsedURL, err := url.Parse(feedURL)
			if err != nil {
				return HandleValidationError(c, "Invalid URL format", "feed_urls", feedURL)
			}

			if err := IsAllowedURL(parsedURL); err != nil {
				securityErr := errors.NewValidationContextError(
					"URL not allowed for security reasons",
					"rest",
					"RESTHandler",
					"fetch_article_summary",
					map[string]interface{}{
						"url":         feedURL,
						"reason":      err.Error(),
						"path":        c.Request().URL.Path,
						"method":      c.Request().Method,
						"remote_addr": c.Request().RemoteAddr,
						"request_id":  c.Response().Header().Get("X-Request-ID"),
					},
				)
				logger.Logger.Error("URL validation failed", "error", securityErr.Error(), "url", feedURL)
				return c.JSON(securityErr.HTTPStatusCode(), securityErr.ToHTTPResponse())
			}
		}

		logger.Logger.Info("Fetching article summaries", "url_count", len(req.FeedURLs))

		// Step 1: Check which articles exist in DB and collect URLs that need fetching
		urlsToFetch := make([]string, 0)
		articleMap := make(map[string]*ArticleInfo) // url -> article info

		for _, feedURL := range req.FeedURLs {
			existingArticle, err := container.AltDBRepository.FetchArticleByURL(c.Request().Context(), feedURL)
			if err != nil {
				logger.Logger.Error("Failed to check for existing article", "error", err, "url", feedURL)
				// Create placeholder for failed lookup
				articleMap[feedURL] = &ArticleInfo{URL: feedURL, Error: err}
				continue
			}

			if existingArticle != nil {
				// Article exists in DB
				logger.Logger.Info("Article found in database", "article_id", existingArticle.ID, "url", feedURL)
				articleMap[feedURL] = &ArticleInfo{
					URL:    feedURL,
					ID:     existingArticle.ID,
					Title:  existingArticle.Title,
					Exists: true,
				}
			} else {
				// Article does not exist, add to fetch list
				urlsToFetch = append(urlsToFetch, feedURL)
				articleMap[feedURL] = &ArticleInfo{URL: feedURL, Exists: false}
			}
		}

		// Step 2: Batch fetch articles that don't exist in DB
		if len(urlsToFetch) > 0 {
			logger.Logger.Info("Fetching articles from Web", "url_count", len(urlsToFetch))
			fetchResults := container.BatchArticleFetcher.FetchMultiple(c.Request().Context(), urlsToFetch)

			// Save fetched articles to DB
			for urlStr, result := range fetchResults {
				if result.Error != nil {
					logger.Logger.Error("Failed to fetch article content", "error", result.Error, "url", urlStr)
					if info, ok := articleMap[urlStr]; ok {
						info.Error = result.Error
					}
					continue
				}

				// Save to DB
				articleID, saveErr := container.AltDBRepository.SaveArticle(c.Request().Context(), urlStr, result.Title, result.Content)
				if saveErr != nil {
					logger.Logger.Error("Failed to save article to database", "error", saveErr, "url", urlStr)
					if info, ok := articleMap[urlStr]; ok {
						info.Error = saveErr
					}
					continue
				}

				// Update article map with fetched data
				if info, ok := articleMap[urlStr]; ok {
					info.ID = articleID
					info.Title = result.Title
					info.Exists = true
				}
			}
		}

		// Step 3: Process each URL for summary generation
		var matchedArticles []InoreaderSummaryResponse
		for _, feedURL := range req.FeedURLs {
			articleInfo, ok := articleMap[feedURL]
			if !ok {
				logger.Logger.Error("Skipping URL: article info not found", "url", feedURL)
				continue
			}
			if articleInfo.Error != nil {
				logger.Logger.Error("Skipping URL due to error", "url", feedURL, "error", articleInfo.Error)
				continue
			}

			if !articleInfo.Exists || articleInfo.ID == "" {
				logger.Logger.Warn("Article not found or ID missing", "url", feedURL)
				continue
			}

			articleID := articleInfo.ID
			articleTitle := articleInfo.Title

			// Try to fetch existing summary from database
			var summary string
			var fromCache bool
			existingSummary, err := container.AltDBRepository.FetchArticleSummaryByArticleID(c.Request().Context(), articleID)
			if err == nil && existingSummary != nil && existingSummary.Summary != "" {
				logger.Logger.Info("Found existing summary in database", "article_id", articleID, "feed_url", feedURL)
				summary = existingSummary.Summary
				fromCache = true
			} else {
				// Generate new summary if not found in database
				logger.Logger.Info("No existing summary found, generating new summary", "article_id", articleID, "feed_url", feedURL)

				// Small delay to ensure DB transaction is committed before pre-processor reads
				time.Sleep(100 * time.Millisecond)

				// Call pre-processor with empty content (it will fetch from DB)
				summary, err = CallPreProcessorSummarize(c.Request().Context(), "", articleID, articleTitle, cfg.PreProcessor.URL)
				if err != nil {
					logger.Logger.Error("Failed to summarize article", "error", err, "url", feedURL, "article_id", articleID)
					continue // Skip this URL and continue with others
				}

				// Save the generated summary to database
				if err := container.AltDBRepository.SaveArticleSummary(c.Request().Context(), articleID, articleTitle, summary); err != nil {
					logger.Logger.Error("Failed to save article summary to database", "error", err, "article_id", articleID, "feed_url", feedURL)
					// Continue even if save fails - we still have the summary to return
				} else {
					logger.Logger.Info("Article summary saved to database", "article_id", articleID, "feed_url", feedURL)
				}
				fromCache = false
			}

			// Clean summary content before adding to response
			cleanedSummary := CleanSummaryContent(summary)

			// Build response item
			matchedArticles = append(matchedArticles, InoreaderSummaryResponse{
				ArticleURL:  feedURL,
				Title:       articleTitle,
				Author:      "", // Author is not available from articles table
				Content:     cleanedSummary,
				ContentType: "text/html",
				PublishedAt: time.Now().Format(time.RFC3339), // Use current time as fallback
				FetchedAt:   time.Now().Format(time.RFC3339),
				InoreaderID: articleID, // Use article_id as source_id
			})

			logger.Logger.Info("Article summary processed", "feed_url", feedURL, "from_cache", fromCache)
		}

		// Build final response
		finalResponse := FeedSummaryProvidedResponse{
			MatchedArticles: matchedArticles,
			TotalMatched:    len(matchedArticles),
			RequestedCount:  len(req.FeedURLs),
		}

		// Set caching headers
		c.Response().Header().Set("Cache-Control", "private, max-age=300") // 5 minutes for generated summaries
		c.Response().Header().Set("Content-Type", "application/json")

		logger.Logger.Info("Article summaries fetched successfully",
			"requested_count", len(req.FeedURLs),
			"matched_count", len(matchedArticles))

		return c.JSON(http.StatusOK, finalResponse)
	}
}
