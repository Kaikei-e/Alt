package validation

import (
	"context"
	"testing"
)

func TestSearchQueryValidator_Validate(t *testing.T) {
	validator := &SearchQueryValidator{}
	ctx := context.Background()

	tests := []struct {
		name     string
		input    interface{}
		expected ValidationResult
	}{
		{
			name: "valid search query",
			input: map[string]interface{}{
				"query": "golang programming",
			},
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name: "valid search query with special characters",
			input: map[string]interface{}{
				"query": "go-lang & programming!",
			},
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name: "missing query field",
			input: map[string]interface{}{
				"other_field": "value",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "query", Message: "query field is required"},
				},
			},
		},
		{
			name: "empty query",
			input: map[string]interface{}{
				"query": "",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "query", Message: "Search query cannot be empty", Value: ""},
				},
			},
		},
		{
			name: "whitespace only query",
			input: map[string]interface{}{
				"query": "   ",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "query", Message: "Search query cannot be empty", Value: "   "},
				},
			},
		},
		{
			name: "query too long",
			input: map[string]interface{}{
				"query": string(make([]byte, 1001)), // 1001 characters
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "query", Message: "Search query too long (maximum 1000 characters)"},
				},
			},
		},
		{
			name: "non-string query",
			input: map[string]interface{}{
				"query": 123,
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "query", Message: "Query must be a string"},
				},
			},
		},
		{
			name:  "non-map input",
			input: "invalid",
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "body", Message: "Request body must be a valid object"},
				},
			},
		},
		{
			name: "query with SQL injection attempt",
			input: map[string]interface{}{
				"query": "'; DROP TABLE feeds; --",
			},
			expected: ValidationResult{
				Valid:  true, // We allow this at validation level, sanitization happens elsewhere
				Errors: nil,
			},
		},
		{
			name: "query exactly at limit",
			input: map[string]interface{}{
				"query": string(make([]byte, 1000)), // Exactly 1000 characters
			},
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.Validate(ctx, tt.input)

			if result.Valid != tt.expected.Valid {
				t.Errorf("Expected valid %v, got %v", tt.expected.Valid, result.Valid)
			}

			if len(result.Errors) != len(tt.expected.Errors) {
				t.Errorf("Expected %d errors, got %d", len(tt.expected.Errors), len(result.Errors))
				return
			}

			for i, expectedErr := range tt.expected.Errors {
				if i >= len(result.Errors) {
					t.Errorf("Missing error at index %d", i)
					continue
				}

				actualErr := result.Errors[i]
				if actualErr.Field != expectedErr.Field {
					t.Errorf("Error %d: Expected field %s, got %s", i, expectedErr.Field, actualErr.Field)
				}
				if actualErr.Message != expectedErr.Message {
					t.Errorf("Error %d: Expected message %s, got %s", i, expectedErr.Message, actualErr.Message)
				}
				if expectedErr.Value != "" && actualErr.Value != expectedErr.Value {
					t.Errorf("Error %d: Expected value %s, got %s", i, expectedErr.Value, actualErr.Value)
				}
			}
		})
	}
}

func TestArticleSearchValidator_Validate(t *testing.T) {
	validator := &ArticleSearchValidator{}
	ctx := context.Background()

	tests := []struct {
		name     string
		input    interface{}
		expected ValidationResult
	}{
		{
			name: "valid article search with query param",
			input: map[string]interface{}{
				"q": "golang programming",
			},
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name: "missing q parameter",
			input: map[string]interface{}{
				"other_param": "value",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "q", Message: "q parameter is required"},
				},
			},
		},
		{
			name: "empty q parameter",
			input: map[string]interface{}{
				"q": "",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "q", Message: "Search query cannot be empty", Value: ""},
				},
			},
		},
		{
			name: "whitespace only q parameter",
			input: map[string]interface{}{
				"q": "   ",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "q", Message: "Search query cannot be empty", Value: "   "},
				},
			},
		},
		{
			name: "q parameter too long",
			input: map[string]interface{}{
				"q": string(make([]byte, 501)), // 501 characters
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "q", Message: "Search query too long (maximum 500 characters)"},
				},
			},
		},
		{
			name: "non-string q parameter",
			input: map[string]interface{}{
				"q": 123,
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "q", Message: "Query parameter must be a string"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.Validate(ctx, tt.input)

			if result.Valid != tt.expected.Valid {
				t.Errorf("Expected valid %v, got %v", tt.expected.Valid, result.Valid)
			}

			if len(result.Errors) != len(tt.expected.Errors) {
				t.Errorf("Expected %d errors, got %d", len(tt.expected.Errors), len(result.Errors))
				return
			}

			for i, expectedErr := range tt.expected.Errors {
				actualErr := result.Errors[i]
				if actualErr.Field != expectedErr.Field {
					t.Errorf("Error %d: Expected field %s, got %s", i, expectedErr.Field, actualErr.Field)
				}
				if actualErr.Message != expectedErr.Message {
					t.Errorf("Error %d: Expected message %s, got %s", i, expectedErr.Message, actualErr.Message)
				}
			}
		})
	}
}
