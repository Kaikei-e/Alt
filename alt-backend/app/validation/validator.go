package validation

import (
	"context"
	"fmt"
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

func ValidateFeedTags(ctx context.Context, feedUrl string) error {
	validator := &FeedURLValidator{}
	result := validator.Validate(ctx, feedUrl)

	if !result.Valid {
		return &ValidationErrorType{
			Type:   "feed_tags_validation",
			Fields: map[string]interface{}{"feed_url": feedUrl, "validation_type": "feed_tags"},
			Errors: result.Errors,
		}
	}
	return nil
}

// ValidateFeedURL validates a feed URL using the FeedRegistrationValidator (includes SSRF protection)
func ValidateFeedURL(ctx context.Context, url string) error {
	validator := &FeedRegistrationValidator{}
	inputMap := map[string]interface{}{"url": url}
	result := validator.Validate(ctx, inputMap)

	if !result.Valid {
		return &ValidationErrorType{
			Type:   "feed_url_validation",
			Fields: map[string]interface{}{"url": url, "validation_type": "feed_url"},
			Errors: result.Errors,
		}
	}
	return nil
}

// ValidateSearchQuery validates a search query using the SearchQueryValidator
func ValidateSearchQuery(ctx context.Context, query string) error {
	validator := &SearchQueryValidator{}
	inputMap := map[string]interface{}{"query": query}
	result := validator.Validate(ctx, inputMap)

	if !result.Valid {
		return &ValidationErrorType{
			Type:   "search_query_validation",
			Fields: map[string]interface{}{"query": query, "validation_type": "search_query"},
			Errors: result.Errors,
		}
	}
	return nil
}

// ValidatePagination validates pagination parameters
func ValidatePagination(ctx context.Context, limit, page int) error {
	result := ValidationResult{Valid: true}

	if limit < 1 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "limit",
			Message: "limit must be positive",
		})
	}

	if limit > 1000 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "limit",
			Message: "limit exceeds maximum",
		})
	}

	if page < 0 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "page",
			Message: "page must be non-negative",
		})
	}

	if !result.Valid {
		return &ValidationErrorType{
			Type:   "pagination_validation",
			Fields: map[string]interface{}{"limit": limit, "page": page, "validation_type": "pagination"},
			Errors: result.Errors,
		}
	}
	return nil
}

// SanitizeInput sanitizes user input by removing potentially harmful content
func SanitizeInput(ctx context.Context, input string) string {
	// Remove script tags and their content
	for {
		start := strings.Index(strings.ToLower(input), "<script")
		if start == -1 {
			break
		}
		end := strings.Index(strings.ToLower(input[start:]), "</script>")
		if end == -1 {
			// No closing tag, remove from start to end
			input = input[:start]
			break
		}
		end += start + len("</script>")
		input = input[:start] + input[end:]
	}

	// Remove any remaining HTML tags
	for {
		start := strings.Index(input, "<")
		if start == -1 {
			break
		}
		end := strings.Index(input[start:], ">")
		if end == -1 {
			// No closing bracket, remove from start to end
			input = input[:start]
			break
		}
		end += start + 1
		input = input[:start] + input[end:]
	}

	// Normalize whitespace
	input = strings.TrimSpace(input)
	input = strings.Join(strings.Fields(input), " ")

	return input
}

// ValidationErrorType represents a typed validation error
type ValidationErrorType struct {
	Type   string                 `json:"type"`
	Fields map[string]interface{} `json:"fields"`
	Errors []ValidationError      `json:"errors"`
}

func (e *ValidationErrorType) Error() string {
	if len(e.Errors) > 0 {
		return e.Errors[0].Message
	}
	return fmt.Sprintf("validation failed: %s", e.Type)
}

// AsValidationError attempts to convert an error to a ValidationErrorType
func AsValidationError(err error) (*ValidationErrorType, bool) {
	if verr, ok := err.(*ValidationErrorType); ok {
		return verr, true
	}
	return nil, false
}
