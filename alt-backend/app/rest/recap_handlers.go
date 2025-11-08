package rest

import (
	"alt/config"
	"alt/di"
	"alt/domain"
	middleware_custom "alt/middleware"
	"alt/usecase/recap_articles_usecase"
	"alt/utils/logger"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

func registerRecapRoutes(v1 *echo.Group, container *di.ApplicationComponents, cfg *config.Config) {
	// Service authentication middleware for internal service-to-service communication
	serviceAuthMiddleware := middleware_custom.NewServiceAuthMiddleware(logger.Logger)

	limiter := newRecapRateLimiter(cfg.Recap.RateLimitRPS, cfg.Recap.RateLimitBurst)

	// Apply service auth middleware to recap routes
	recap := v1.Group("/recap", serviceAuthMiddleware.RequireServiceAuth())
	recap.GET("/articles", handleRecapArticles(container, cfg, limiter))

	// 7-day recap endpoint (publicly accessible)
	recapHandler := NewRecapHandler(container.RecapUsecase)
	v1.GET("/recap/7days", recapHandler.GetSevenDayRecap)
}

func handleRecapArticles(container *di.ApplicationComponents, cfg *config.Config, limiter *recapRateLimiter) echo.HandlerFunc {
	return func(c echo.Context) error {
		limit := cfg.Recap.RateLimitBurst
		allowed, remaining, reset := limiter.Allow(time.Now())
		setRateLimitHeaders(c, limit, remaining, reset)
		if !allowed {
			retryAfter := int(time.Until(reset).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Response().Header().Set("Retry-After", strconv.Itoa(retryAfter))
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
		}

		fromStr := c.QueryParam("from")
		if strings.TrimSpace(fromStr) == "" {
			return handleValidationError(c, "from is required", "from", fromStr)
		}
		toStr := c.QueryParam("to")
		if strings.TrimSpace(toStr) == "" {
			return handleValidationError(c, "to is required", "to", toStr)
		}

		from, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return handleValidationError(c, "from must be RFC3339", "from", fromStr)
		}
		to, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			return handleValidationError(c, "to must be RFC3339", "to", toStr)
		}

		page, err := parsePositiveIntWithDefault(c.QueryParam("page"), 1)
		if err != nil {
			return handleValidationError(c, "page must be a positive integer", "page", c.QueryParam("page"))
		}
		pageSize, err := parsePositiveIntWithDefault(c.QueryParam("page_size"), cfg.Recap.DefaultPageSize)
		if err != nil {
			return handleValidationError(c, "page_size must be a positive integer", "page_size", c.QueryParam("page_size"))
		}

		fields := parseFields(c.QueryParam("fields"))
		langParam := strings.TrimSpace(c.QueryParam("lang"))
		var langHint *string
		if langParam != "" {
			langLower := strings.ToLower(langParam)
			langHint = &langLower
		}

		input := recap_articles_usecase.Input{
			From:     from.UTC(),
			To:       to.UTC(),
			Page:     page,
			PageSize: pageSize,
			LangHint: langHint,
			Fields:   fields,
		}

		result, err := container.RecapArticlesUsecase.Execute(c.Request().Context(), input)
		if err != nil {
			return handleError(c, err, "recap_articles")
		}
		if result == nil {
			result = &domain.RecapArticlesPage{
				Page:     page,
				PageSize: pageSize,
			}
		}

		response := RecapArticlesResponse{
			Range: RecapRangeResponse{
				From: from.UTC().Format(time.RFC3339),
				To:   to.UTC().Format(time.RFC3339),
			},
			Total:    result.Total,
			Page:     result.Page,
			PageSize: result.PageSize,
			HasMore:  result.HasMore,
			Articles: make([]RecapArticleResponse, len(result.Articles)),
		}

		for i, article := range result.Articles {
			response.Articles[i] = mapRecapArticle(article)
		}

		return c.JSON(http.StatusOK, response)
	}
}

func parsePositiveIntWithDefault(value string, def int) (int, error) {
	if strings.TrimSpace(value) == "" {
		return def, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid integer")
	}
	return parsed, nil
}

func parseFields(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		clean := strings.TrimSpace(p)
		if clean == "" {
			continue
		}
		result = append(result, clean)
	}
	return result
}

func mapRecapArticle(article domain.RecapArticle) RecapArticleResponse {
	resp := RecapArticleResponse{
		ArticleID: article.ID.String(),
		FullText:  article.FullText,
	}
	if article.Title != nil {
		resp.Title = article.Title
	}
	if article.SourceURL != nil {
		resp.SourceURL = article.SourceURL
	}
	if article.PublishedAt != nil {
		formatted := article.PublishedAt.UTC().Format(time.RFC3339)
		resp.PublishedAt = &formatted
	}
	if article.LangHint != nil {
		resp.LangHint = article.LangHint
	}
	return resp
}

func setRateLimitHeaders(c echo.Context, limit int, remaining int, reset time.Time) {
	c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	if remaining < 0 {
		remaining = 0
	}
	c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	c.Response().Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
}

type recapRateLimiter struct {
	mu        sync.Mutex
	tokens    float64
	capacity  float64
	fillRate  float64
	lastCheck time.Time
}

func newRecapRateLimiter(rps, burst int) *recapRateLimiter {
	if rps <= 0 {
		rps = 1
	}
	if burst <= 0 {
		burst = rps
	}
	now := time.Now()
	return &recapRateLimiter{
		tokens:    float64(burst),
		capacity:  float64(burst),
		fillRate:  float64(rps),
		lastCheck: now,
	}
}

func (r *recapRateLimiter) Allow(now time.Time) (bool, int, time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	elapsed := now.Sub(r.lastCheck).Seconds()
	if elapsed > 0 {
		r.tokens = math.Min(r.capacity, r.tokens+elapsed*r.fillRate)
	}
	r.lastCheck = now
	if r.tokens < 1 {
		needed := (1 - r.tokens) / r.fillRate
		reset := now.Add(time.Duration(math.Ceil(needed * float64(time.Second))))
		return false, int(math.Floor(r.tokens)), reset
	}
	r.tokens -= 1
	remaining := int(math.Floor(r.tokens))
	resetDelay := (r.capacity - r.tokens) / r.fillRate
	reset := now.Add(time.Duration(math.Ceil(resetDelay * float64(time.Second))))
	return true, remaining, reset
}
