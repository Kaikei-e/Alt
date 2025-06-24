package validation

import (
	"context"
	"testing"
)

func TestValidationError(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		message  string
		value    string
		expected ValidationError
	}{
		{
			name:    "basic validation error",
			field:   "url",
			message: "URL is required",
			value:   "",
			expected: ValidationError{
				Field:   "url",
				Message: "URL is required",
				Value:   "",
			},
		},
		{
			name:    "validation error with complex value",
			field:   "email",
			message: "Invalid email format",
			value:   "invalid-email",
			expected: ValidationError{
				Field:   "email",
				Message: "Invalid email format",
				Value:   "invalid-email",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidationError{
				Field:   tt.field,
				Message: tt.message,
				Value:   tt.value,
			}

			if err.Field != tt.expected.Field {
				t.Errorf("Expected field %s, got %s", tt.expected.Field, err.Field)
			}
			if err.Message != tt.expected.Message {
				t.Errorf("Expected message %s, got %s", tt.expected.Message, err.Message)
			}
			if err.Value != tt.expected.Value {
				t.Errorf("Expected value %s, got %s", tt.expected.Value, err.Value)
			}
		})
	}
}

func TestValidationResult(t *testing.T) {
	tests := []struct {
		name   string
		result ValidationResult
		valid  bool
		errors []ValidationError
	}{
		{
			name: "valid result",
			result: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
			valid:  true,
			errors: nil,
		},
		{
			name: "invalid result with errors",
			result: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "URL is required"},
				},
			},
			valid: false,
			errors: []ValidationError{
				{Field: "url", Message: "URL is required"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.Valid != tt.valid {
				t.Errorf("Expected valid %v, got %v", tt.valid, tt.result.Valid)
			}
			if len(tt.result.Errors) != len(tt.errors) {
				t.Errorf("Expected %d errors, got %d", len(tt.errors), len(tt.result.Errors))
			}
		})
	}
}

func TestFeedURLValidator_Validate(t *testing.T) {
	validator := &FeedURLValidator{}
	ctx := context.Background()

	tests := []struct {
		name     string
		input    interface{}
		expected ValidationResult
	}{
		{
			name:  "valid HTTP URL",
			input: "http://example.com/feed.xml",
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name:  "valid HTTPS URL",
			input: "https://example.com/feed.xml",
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name:  "empty string",
			input: "",
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "URL cannot be empty", Value: ""},
				},
			},
		},
		{
			name:  "whitespace only",
			input: "   ",
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "URL cannot be empty", Value: "   "},
				},
			},
		},
		{
			name:  "invalid URL format",
			input: "not-a-url",
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "Invalid URL format", Value: "not-a-url"},
				},
			},
		},
		{
			name:  "URL with invalid scheme",
			input: "ftp://example.com/feed.xml",
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "URL must use HTTP or HTTPS scheme", Value: "ftp://example.com/feed.xml"},
				},
			},
		},
		{
			name:  "non-string input",
			input: 123,
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "URL must be a string"},
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