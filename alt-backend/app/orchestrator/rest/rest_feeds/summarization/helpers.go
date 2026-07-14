package summarization

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"alt/di"
	"alt/utils/errors"
	"alt/utils/html_parser"
	"alt/utils/logger"

	"github.com/labstack/echo/v4"
)

func respondWithSummary(c echo.Context, summary, articleID, feedURL string) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":    true,
		"summary":    summary,
		"article_id": articleID,
		"feed_url":   feedURL,
	})
}

func handleError(c echo.Context, err error, operation string) error {
	ctx := c.Request().Context()
	var enrichedErr *errors.AppContextError

	if appContextErr, ok := err.(*errors.AppContextError); ok {
		enrichedErr = errors.EnrichWithContext(appContextErr, "summarization", "RESTHandler", operation, map[string]interface{}{
			"path":        c.Request().URL.Path,
			"method":      c.Request().Method,
			"remote_addr": c.Request().RemoteAddr,
			"user_agent":  c.Request().UserAgent(),
			"request_id":  c.Response().Header().Get("X-Request-ID"),
		})
	} else if appErr, ok := err.(*errors.AppError); ok {
		enrichedErr = errors.NewAppContextError(
			string(appErr.Code),
			appErr.Message,
			"summarization",
			"RESTHandler",
			operation,
			appErr.Cause,
			map[string]interface{}{
				"path":           c.Request().URL.Path,
				"method":         c.Request().Method,
				"remote_addr":    c.Request().RemoteAddr,
				"user_agent":     c.Request().UserAgent(),
				"request_id":     c.Response().Header().Get("X-Request-ID"),
				"legacy_context": appErr.Context,
			},
		)
	} else {
		enrichedErr = errors.NewUnknownContextError(
			"internal server error",
			"summarization",
			"RESTHandler",
			operation,
			err,
			map[string]interface{}{
				"path":        c.Request().URL.Path,
				"method":      c.Request().Method,
				"remote_addr": c.Request().RemoteAddr,
				"user_agent":  c.Request().UserAgent(),
				"request_id":  c.Response().Header().Get("X-Request-ID"),
			},
		)
	}

	logger.Logger.ErrorContext(ctx, "REST API Error", "error", enrichedErr.Error(), "code", enrichedErr.Code, "operation", operation, "path", c.Request().URL.Path)

	return c.JSON(enrichedErr.HTTPStatusCode(), map[string]interface{}{
		"error": map[string]interface{}{
			"code":    enrichedErr.Code,
			"message": enrichedErr.Message,
		},
	})
}

func handleValidationError(c echo.Context, message, field string, value interface{}) error {
	ctx := c.Request().Context()
	logger.Logger.WarnContext(ctx, "Validation error", "message", message, "field", field, "value", value)
	return c.JSON(http.StatusBadRequest, map[string]interface{}{
		"error": message,
		"field": field,
		"value": value,
		"code":  "VALIDATION_ERROR",
	})
}

// fetchArticleContent fetches urlStr via the SSRF-safe article fetch gateway
// (the same driver-adjacent gateway used by ArticleUsecase and
// BatchArticleFetcher) and extracts its title and readable text. Falls back
// to raw HTML when text extraction yields nothing.
func fetchArticleContent(ctx context.Context, urlStr string, container *di.ApplicationComponents) (content string, title string, err error) {
	contentPtr, err := container.FetchArticleGateway.FetchArticleContents(ctx, urlStr)
	if err != nil {
		return "", "", fmt.Errorf("fetch article content: %w", err)
	}

	htmlContent := ""
	if contentPtr != nil {
		htmlContent = *contentPtr
	}

	extractedTitle := html_parser.ExtractTitle(htmlContent)
	extractedText := html_parser.ExtractArticleText(htmlContent)
	if extractedText == "" {
		logger.Logger.WarnContext(ctx, "failed to extract article text from HTML, falling back to raw HTML",
			"url", urlStr, "html_size_bytes", len(htmlContent))
		return htmlContent, extractedTitle, nil
	}

	return extractedText, extractedTitle, nil
}

// streamPreProcessorSummarize streams a summary from the pre-processor via
// the shared driver-backed gateway.
func streamPreProcessorSummarize(ctx context.Context, container *di.ApplicationComponents, content, articleID, title string) (io.ReadCloser, error) {
	stream, err := container.PreProcessorSummarizeGateway.StreamSummarize(ctx, content, articleID, title)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Pre-processor stream failed",
			"error", err,
			"article_id", articleID)
		return nil, err
	}
	return stream, nil
}
