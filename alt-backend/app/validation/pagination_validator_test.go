package validation

import (
	"context"
	"testing"
)

func TestPaginationValidator_Validate(t *testing.T) {
	validator := &PaginationValidator{}
	ctx := context.Background()

	tests := []struct {
		name     string
		input    interface{}
		expected ValidationResult
	}{
		{
			name: "valid pagination with limit only",
			input: map[string]interface{}{
				"limit": "20",
			},
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name: "valid pagination with page only",
			input: map[string]interface{}{
				"page": "1",
			},
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name: "valid pagination with cursor only",
			input: map[string]interface{}{
				"cursor": "2023-01-01T00:00:00Z",
			},
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name: "valid pagination with limit and cursor",
			input: map[string]interface{}{
				"limit":  "50",
				"cursor": "2023-01-01T00:00:00Z",
			},
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name:     "valid empty pagination",
			input:    map[string]interface{}{},
			expected: ValidationResult{Valid: true, Errors: nil},
		},
		{
			name: "invalid limit - negative",
			input: map[string]interface{}{
				"limit": "-1",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "limit", Message: "Limit must be a positive integer", Value: "-1"},
				},
			},
		},
		{
			name: "invalid limit - zero",
			input: map[string]interface{}{
				"limit": "0",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "limit", Message: "Limit must be a positive integer", Value: "0"},
				},
			},
		},
		{
			name: "invalid limit - too large",
			input: map[string]interface{}{
				"limit": "1001",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "limit", Message: "Limit too large (maximum 1000)", Value: "1001"},
				},
			},
		},
		{
			name: "invalid limit - non-numeric",
			input: map[string]interface{}{
				"limit": "abc",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "limit", Message: "Limit must be a valid integer", Value: "abc"},
				},
			},
		},
		{
			name: "invalid page - negative",
			input: map[string]interface{}{
				"page": "-1",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "page", Message: "Page must be a non-negative integer", Value: "-1"},
				},
			},
		},
		{
			name: "invalid page - non-numeric",
			input: map[string]interface{}{
				"page": "abc",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "page", Message: "Page must be a valid integer", Value: "abc"},
				},
			},
		},
		{
			name: "invalid cursor - malformed RFC3339",
			input: map[string]interface{}{
				"cursor": "2023-01-01 00:00:00",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "cursor", Message: "Cursor must be a valid RFC3339 timestamp", Value: "2023-01-01 00:00:00"},
				},
			},
		},
		{
			name: "invalid cursor - completely invalid",
			input: map[string]interface{}{
				"cursor": "invalid-timestamp",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "cursor", Message: "Cursor must be a valid RFC3339 timestamp", Value: "invalid-timestamp"},
				},
			},
		},
		{
			name: "non-string limit",
			input: map[string]interface{}{
				"limit": 20,
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "limit", Message: "Limit parameter must be a string"},
				},
			},
		},
		{
			name: "non-string page",
			input: map[string]interface{}{
				"page": 1,
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "page", Message: "Page parameter must be a string"},
				},
			},
		},
		{
			name: "non-string cursor",
			input: map[string]interface{}{
				"cursor": 1234567890,
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "cursor", Message: "Cursor parameter must be a string"},
				},
			},
		},
		{
			name:  "non-map input",
			input: "invalid",
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "params", Message: "Query parameters must be a valid object"},
				},
			},
		},
		{
			name: "multiple validation errors",
			input: map[string]interface{}{
				"limit":  "-5",
				"page":   "abc",
				"cursor": "invalid",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "limit", Message: "Limit must be a positive integer", Value: "-5"},
					{Field: "page", Message: "Page must be a valid integer", Value: "abc"},
					{Field: "cursor", Message: "Cursor must be a valid RFC3339 timestamp", Value: "invalid"},
				},
			},
		},
		{
			name: "limit at maximum boundary",
			input: map[string]interface{}{
				"limit": "1000",
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
				for i, err := range result.Errors {
					t.Logf("  Error %d: %s - %s", i, err.Field, err.Message)
				}
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