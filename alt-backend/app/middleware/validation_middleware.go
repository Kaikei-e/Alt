package middleware

import (
	"alt/validation"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func ValidationMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Validate based on route and method
			if err := validateRoute(c); err != nil {
				return err
			}
			
			return next(c)
		}
	}
}

func validateRoute(c echo.Context) error {
	path := c.Request().URL.Path
	method := c.Request().Method
	
	switch {
	case method == "POST" && strings.Contains(path, "/rss-feed-link/register"):
		return validateFeedRegistration(c)
	case method == "POST" && strings.Contains(path, "/feeds/search"):
		return validateFeedSearch(c)
	case method == "POST" && strings.Contains(path, "/feeds/fetch/details"):
		return validateFeedDetails(c)
	case method == "GET" && strings.Contains(path, "/articles/search"):
		return validateArticleSearch(c)
	case method == "GET" && (strings.Contains(path, "/feeds/fetch/cursor") ||
		strings.Contains(path, "/feeds/fetch/limit") ||
		strings.Contains(path, "/feeds/fetch/page")):
		return validatePagination(c)
	default:
		return nil // No validation needed
	}
}

func validateFeedRegistration(c echo.Context) error {
	// Read and parse request body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Failed to read request body",
		})
	}
	
	// Reset body for further processing
	c.Request().Body = io.NopCloser(strings.NewReader(string(body)))
	
	var requestData map[string]interface{}
	if err := json.Unmarshal(body, &requestData); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Invalid JSON format",
		})
	}
	
	validator := &validation.FeedRegistrationValidator{}
	result := validator.Validate(c.Request().Context(), requestData)
	
	if !result.Valid {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Validation failed",
			"details": result.Errors,
		})
	}
	
	return nil
}

func validateFeedSearch(c echo.Context) error {
	// Read and parse request body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Failed to read request body",
		})
	}
	
	// Reset body for further processing
	c.Request().Body = io.NopCloser(strings.NewReader(string(body)))
	
	var requestData map[string]interface{}
	if err := json.Unmarshal(body, &requestData); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Invalid JSON format",
		})
	}
	
	validator := &validation.SearchQueryValidator{}
	result := validator.Validate(c.Request().Context(), requestData)
	
	if !result.Valid {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Validation failed",
			"details": result.Errors,
		})
	}
	
	return nil
}

func validateFeedDetails(c echo.Context) error {
	// Read and parse request body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Failed to read request body",
		})
	}
	
	// Reset body for further processing
	c.Request().Body = io.NopCloser(strings.NewReader(string(body)))
	
	var requestData map[string]interface{}
	if err := json.Unmarshal(body, &requestData); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Invalid JSON format",
		})
	}
	
	validator := &validation.FeedDetailValidator{}
	result := validator.Validate(c.Request().Context(), requestData)
	
	if !result.Valid {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Validation failed",
			"details": result.Errors,
		})
	}
	
	return nil
}

func validateArticleSearch(c echo.Context) error {
	// Get query parameters
	queryParams := make(map[string]interface{})
	for key, values := range c.Request().URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}
	
	validator := &validation.ArticleSearchValidator{}
	result := validator.Validate(c.Request().Context(), queryParams)
	
	if !result.Valid {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Validation failed",
			"details": result.Errors,
		})
	}
	
	return nil
}

func validatePagination(c echo.Context) error {
	// Get query parameters
	queryParams := make(map[string]interface{})
	for key, values := range c.Request().URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}
	
	validator := &validation.PaginationValidator{}
	result := validator.Validate(c.Request().Context(), queryParams)
	
	if !result.Valid {
		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":   "validation_failed",
			"message": "Validation failed",
			"details": result.Errors,
		})
	}
	
	return nil
}