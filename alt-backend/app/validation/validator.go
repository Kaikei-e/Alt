package validation

import (
	"context"
	"net/url"
	"strings"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Value   string `json:"value,omitempty"`
}

type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

type Validator interface {
	Validate(ctx context.Context, value interface{}) ValidationResult
}

type FeedURLValidator struct{}

func (v *FeedURLValidator) Validate(ctx context.Context, value interface{}) ValidationResult {
	result := ValidationResult{Valid: true}
	
	urlStr, ok := value.(string)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "URL must be a string",
		})
		return result
	}
	
	if strings.TrimSpace(urlStr) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "URL cannot be empty",
			Value:   urlStr,
		})
		return result
	}
	
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "Invalid URL format",
			Value:   urlStr,
		})
		return result
	}
	
	// Check for valid scheme first, but treat empty scheme as invalid format
	if parsedURL.Scheme == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "Invalid URL format",
			Value:   urlStr,
		})
		return result
	}
	
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "url",
			Message: "URL must use HTTP or HTTPS scheme",
			Value:   urlStr,
		})
		return result
	}
	
	return result
}