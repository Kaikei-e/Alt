package rest

import (
	"alt/di"
	"alt/domain"
	"alt/orchestrator/rest/resterr"
	"alt/orchestrator/usecase/archive_article_usecase"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"alt/utils/security"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
)

func handleArchiveArticle(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		var payload ArchiveArticleRequest
		if err := c.Bind(&payload); err != nil {
			return resterr.HandleValidationError(c, "Invalid request format", "body", "malformed JSON")
		}

		if strings.TrimSpace(payload.FeedURL) == "" {
			return resterr.HandleValidationError(c, "Article URL is required", "feed_url", payload.FeedURL)
		}

		articleURL, err := url.Parse(payload.FeedURL)
		if err != nil {
			return resterr.HandleValidationError(c, "Invalid article URL", "feed_url", payload.FeedURL)
		}

		if err := security.NewURLSecurityValidator().ValidateParsedRSSURL(articleURL); err != nil {
			return resterr.HandleValidationError(c, "Article URL not allowed", "feed_url", payload.FeedURL)
		}

		input := archive_article_usecase.ArchiveArticleInput{
			URL:   articleURL.String(),
			Title: payload.Title,
		}

		if err := container.ArchiveArticleUsecase.Execute(c.Request().Context(), input); err != nil {
			return resterr.HandleError(c, fmt.Errorf("archive article failed for %q: %w", articleURL.String(), err), "archive_article")
		}

		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.JSON(http.StatusOK, map[string]string{"message": "article archived"})
	}
}

func handleFetchArticle(container *di.ApplicationComponents) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		targetURL := c.QueryParam("url")
		parsedURL, err := validateFetchRequest(c, targetURL)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}

		user, err := domain.GetUserFromContext(ctx)
		if err != nil {
			logger.Logger.WarnContext(ctx, "No user context for fetch article, proceeding as anonymous", "error", err)
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		}

		// Call the usecase
		content, articleID, _, err := container.ArticleUsecase.FetchCompliantArticle(ctx, parsedURL, *user)
		if err != nil {
			var complianceErr *domain.ComplianceError
			if errors.As(err, &complianceErr) {
				return c.JSON(complianceErr.Code, map[string]string{"error": complianceErr.Message})
			}

			if errors.Is(err, context.DeadlineExceeded) {
				return c.JSON(http.StatusGatewayTimeout, map[string]string{"error": "Request timeout"})
			}
			logger.Logger.ErrorContext(ctx, "Failed to fetch compliant article", "error", err, "url", targetURL)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch article"})
		}

		return returnArticleResponse(c, parsedURL, content, articleID)
	}
}

func validateFetchRequest(c echo.Context, targetURL string) (*url.URL, error) {
	if targetURL == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}

	if err := security.NewURLSecurityValidator().ValidateParsedRSSURL(parsedURL); err != nil {
		return nil, fmt.Errorf("invalid URL scheme or private IP blocked")
	}

	return parsedURL, nil
}

func returnArticleResponse(c echo.Context, articleURL *url.URL, content string, articleID string) error {
	escapedContent := html_parser.StripTags(content)
	return c.JSON(http.StatusOK, map[string]string{
		"url":        articleURL.String(),
		"content":    escapedContent,
		"article_id": articleID,
	})
}
