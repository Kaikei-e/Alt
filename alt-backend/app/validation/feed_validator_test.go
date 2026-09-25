package validation

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestFeedRegistrationValidator_Validate(t *testing.T) {
	validator := &FeedRegistrationValidator{}
	ctx := context.Background()

	tests := []struct {
		name     string
		input    interface{}
		expected ValidationResult
	}{
		{
			name: "valid feed registration request",
			input: map[string]interface{}{
				"url": "https://example.com/feed.xml",
			},
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name: "missing URL field",
			input: map[string]interface{}{
				"other_field": "value",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "URL field is required"},
				},
			},
		},
		{
			name: "empty URL",
			input: map[string]interface{}{
				"url": "",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "URL cannot be empty", Value: ""},
				},
			},
		},
		{
			name: "invalid URL format",
			input: map[string]interface{}{
				"url": "not-a-url",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "Invalid URL format", Value: "not-a-url"},
				},
			},
		},
		{
			name: "URL with invalid scheme",
			input: map[string]interface{}{
				"url": "ftp://example.com/feed.xml",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "URL must use HTTP or HTTPS scheme", Value: "ftp://example.com/feed.xml"},
				},
			},
		},
		{
			name: "localhost URL (SSRF protection)",
			input: map[string]interface{}{
				"url": "http://localhost:8080/feed.xml",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "Access to localhost not allowed for security reasons", Value: "http://localhost:8080/feed.xml"},
				},
			},
		},
		{
			name: "private IP URL (SSRF protection)",
			input: map[string]interface{}{
				"url": "http://192.168.1.1/feed.xml",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "Access to private networks not allowed for security reasons", Value: "http://192.168.1.1/feed.xml"},
				},
			},
		},
		{
			name: "metadata endpoint URL (SSRF protection)",
			input: map[string]interface{}{
				"url": "http://169.254.169.254/metadata",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "Access to metadata endpoints not allowed for security reasons", Value: "http://169.254.169.254/metadata"},
				},
			},
		},
		{
			name: "non-string URL field",
			input: map[string]interface{}{
				"url": 123,
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "url", Message: "URL must be a string"},
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

func TestFeedRegistrationValidator_Validate_AllowsConfiguredFeedHosts(t *testing.T) {
	t.Setenv("FEED_ALLOWED_HOSTS", "mock-rss-001,mock-rss-002")

	validator := &FeedRegistrationValidator{}
	ctx := context.Background()

	result := validator.Validate(ctx, map[string]interface{}{
		"url": "http://mock-rss-001/feed.xml",
	})

	if !result.Valid {
		t.Fatalf("expected configured feed host to be allowed, got errors: %+v", result.Errors)
	}
}

func TestFeedDetailValidator_Validate(t *testing.T) {
	validator := &FeedDetailValidator{}
	ctx := context.Background()

	tests := []struct {
		name     string
		input    interface{}
		expected ValidationResult
	}{
		{
			name: "valid feed detail request",
			input: map[string]interface{}{
				"feed_url": "https://example.com/feed.xml",
			},
			expected: ValidationResult{
				Valid:  true,
				Errors: nil,
			},
		},
		{
			name: "missing feed_url field",
			input: map[string]interface{}{
				"other_field": "value",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "feed_url", Message: "feed_url field is required"},
				},
			},
		},
		{
			name: "empty feed_url",
			input: map[string]interface{}{
				"feed_url": "",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "feed_url", Message: "Feed URL cannot be empty", Value: ""},
				},
			},
		},
		{
			name: "invalid feed_url format",
			input: map[string]interface{}{
				"feed_url": "not-a-url",
			},
			expected: ValidationResult{
				Valid: false,
				Errors: []ValidationError{
					{Field: "feed_url", Message: "Invalid feed URL format", Value: "not-a-url"},
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

func TestFeedRegistrationValidator_InjectedResolver(t *testing.T) {
	ctx := context.Background()

	// Test injected resolver returning private IP (blocked)
	validatorBlocked := &FeedRegistrationValidator{
		Resolver: func(host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("192.168.1.50")}, nil
		},
	}
	resultBlocked := validatorBlocked.Validate(ctx, map[string]interface{}{
		"url": "https://custom-feed-domain.com/rss",
	})
	if resultBlocked.Valid {
		t.Fatalf("expected private IP from resolver to be blocked")
	}

	// Test injected resolver returning public IP (allowed)
	validatorAllowed := &FeedRegistrationValidator{
		Resolver: func(host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		},
	}
	resultAllowed := validatorAllowed.Validate(ctx, map[string]interface{}{
		"url": "https://custom-feed-domain.com/rss",
	})
	if !resultAllowed.Valid {
		t.Fatalf("expected public IP from resolver to be allowed, got errors: %+v", resultAllowed.Errors)
	}

	// Test injected resolver failing on non-common TLD (blocked)
	validatorUncommonTLD := &FeedRegistrationValidator{
		Resolver: func(host string) ([]net.IP, error) {
			return nil, errors.New("dns failure")
		},
	}
	resultUncommon := validatorUncommonTLD.Validate(ctx, map[string]interface{}{
		"url": "https://custom-feed-domain.xyz/rss",
	})
	if resultUncommon.Valid {
		t.Fatalf("expected DNS failure on uncommon TLD to be blocked")
	}
}

func TestFeedRegistrationValidator_ConsolidatedMetadata(t *testing.T) {
	ctx := context.Background()
	validator := &FeedRegistrationValidator{}

	metadataURLs := []string{
		"http://169.254.169.254/feed.xml",
		"http://metadata.google.internal/feed.xml",
		"http://100.100.100.200/feed.xml",
		"http://192.0.0.192/feed.xml",
	}

	for _, u := range metadataURLs {
		result := validator.Validate(ctx, map[string]interface{}{"url": u})
		if result.Valid {
			t.Errorf("expected metadata URL %s to be blocked", u)
		}
	}
}
