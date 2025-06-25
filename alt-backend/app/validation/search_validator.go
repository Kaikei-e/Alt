package validation

import (
	"context"
	"strings"
)

type SearchQueryValidator struct{}

func (v *SearchQueryValidator) Validate(ctx context.Context, value interface{}) ValidationResult {
	result := ValidationResult{Valid: true}

	// Check if input is a map (JSON object)
	inputMap, ok := value.(map[string]interface{})
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "body",
			Message: "Request body must be a valid object",
		})
		return result
	}

	// Check if query field exists
	queryField, exists := inputMap["query"]
	if !exists {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "query",
			Message: "query field is required",
		})
		return result
	}

	// Check if query is a string
	queryStr, ok := queryField.(string)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "query",
			Message: "Query must be a string",
		})
		return result
	}

	// Check if query is empty or whitespace only
	if strings.TrimSpace(queryStr) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "query",
			Message: "Search query cannot be empty",
			Value:   queryStr,
		})
		return result
	}

	// Check query length (maximum 1000 characters)
	if len(queryStr) > 1000 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "query",
			Message: "Search query too long (maximum 1000 characters)",
		})
		return result
	}

	return result
}

type ArticleSearchValidator struct{}

func (v *ArticleSearchValidator) Validate(ctx context.Context, value interface{}) ValidationResult {
	result := ValidationResult{Valid: true}

	// Check if input is a map (query parameters)
	inputMap, ok := value.(map[string]interface{})
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "params",
			Message: "Query parameters must be a valid object",
		})
		return result
	}

	// Check if q parameter exists
	qField, exists := inputMap["q"]
	if !exists {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "q",
			Message: "q parameter is required",
		})
		return result
	}

	// Check if q is a string
	qStr, ok := qField.(string)
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "q",
			Message: "Query parameter must be a string",
		})
		return result
	}

	// Check if q is empty or whitespace only
	if strings.TrimSpace(qStr) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "q",
			Message: "Search query cannot be empty",
			Value:   qStr,
		})
		return result
	}

	// Check query length (maximum 500 characters for GET parameters)
	if len(qStr) > 500 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "q",
			Message: "Search query too long (maximum 500 characters)",
		})
		return result
	}

	return result
}
