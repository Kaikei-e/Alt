package validation

import (
	"context"
	"strconv"
	"strings"
	"time"
)

type PaginationValidator struct{}

func (v *PaginationValidator) Validate(ctx context.Context, value interface{}) ValidationResult {
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
	
	// Validate limit parameter if present
	if limitField, exists := inputMap["limit"]; exists {
		if err := v.validateLimit(limitField); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, *err)
		}
	}
	
	// Validate page parameter if present
	if pageField, exists := inputMap["page"]; exists {
		if err := v.validatePage(pageField); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, *err)
		}
	}
	
	// Validate cursor parameter if present
	if cursorField, exists := inputMap["cursor"]; exists {
		if err := v.validateCursor(cursorField); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, *err)
		}
	}
	
	return result
}

func (v *PaginationValidator) validateLimit(limitField interface{}) *ValidationError {
	// Check if limit is a string
	limitStr, ok := limitField.(string)
	if !ok {
		return &ValidationError{
			Field:   "limit",
			Message: "Limit parameter must be a string",
		}
	}
	
	// Parse limit as integer
	limit, err := strconv.Atoi(strings.TrimSpace(limitStr))
	if err != nil {
		return &ValidationError{
			Field:   "limit",
			Message: "Limit must be a valid integer",
			Value:   limitStr,
		}
	}
	
	// Check if limit is positive
	if limit <= 0 {
		return &ValidationError{
			Field:   "limit",
			Message: "Limit must be a positive integer",
			Value:   limitStr,
		}
	}
	
	// Check if limit is not too large
	if limit > 1000 {
		return &ValidationError{
			Field:   "limit",
			Message: "Limit too large (maximum 1000)",
			Value:   limitStr,
		}
	}
	
	return nil
}

func (v *PaginationValidator) validatePage(pageField interface{}) *ValidationError {
	// Check if page is a string
	pageStr, ok := pageField.(string)
	if !ok {
		return &ValidationError{
			Field:   "page",
			Message: "Page parameter must be a string",
		}
	}
	
	// Parse page as integer
	page, err := strconv.Atoi(strings.TrimSpace(pageStr))
	if err != nil {
		return &ValidationError{
			Field:   "page",
			Message: "Page must be a valid integer",
			Value:   pageStr,
		}
	}
	
	// Check if page is non-negative
	if page < 0 {
		return &ValidationError{
			Field:   "page",
			Message: "Page must be a non-negative integer",
			Value:   pageStr,
		}
	}
	
	return nil
}

func (v *PaginationValidator) validateCursor(cursorField interface{}) *ValidationError {
	// Check if cursor is a string
	cursorStr, ok := cursorField.(string)
	if !ok {
		return &ValidationError{
			Field:   "cursor",
			Message: "Cursor parameter must be a string",
		}
	}
	
	// Check if cursor is empty (empty is valid - means start from beginning)
	if strings.TrimSpace(cursorStr) == "" {
		return nil
	}
	
	// Validate cursor as RFC3339 timestamp
	_, err := time.Parse(time.RFC3339, cursorStr)
	if err != nil {
		return &ValidationError{
			Field:   "cursor",
			Message: "Cursor must be a valid RFC3339 timestamp",
			Value:   cursorStr,
		}
	}
	
	return nil
}