package errors

import (
	"errors"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name          string
		sentinelError error
		wantMessage   string
	}{
		{"ErrFeedNotFound", ErrFeedNotFound, "feed not found"},
		{"ErrDatabaseUnavailable", ErrDatabaseUnavailable, "database unavailable"},
		{"ErrRateLimitExceeded", ErrRateLimitExceeded, "rate limit exceeded"},
		{"ErrExternalServiceUnavailable", ErrExternalServiceUnavailable, "external service unavailable"},
		{"ErrOperationTimeout", ErrOperationTimeout, "operation timeout"},
		{"ErrInvalidInput", ErrInvalidInput, "invalid input"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sentinelError.Error() != tt.wantMessage {
				t.Errorf("%s.Error() = %v, want %v", tt.name, tt.sentinelError.Error(), tt.wantMessage)
			}
		})
	}
}

func TestIsFeedNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "direct sentinel error",
			err:  ErrFeedNotFound,
			want: true,
		},
		{
			name: "wrapped sentinel error",
			err:  NewFeedNotFoundError("gateway", "FeedGateway", "GetFeed", map[string]interface{}{"id": 123}),
			want: true,
		},
		{
			name: "different error",
			err:  ErrDatabaseUnavailable,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsFeedNotFound(tt.err)
			if got != tt.want {
				t.Errorf("IsFeedNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDatabaseError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "direct database error",
			err:  ErrDatabaseUnavailable,
			want: true,
		},
		{
			name: "wrapped database error",
			err:  NewDatabaseUnavailableError("gateway", "DBGateway", "Connect", nil, map[string]interface{}{"host": "localhost"}),
			want: true,
		},
		{
			name: "different error",
			err:  ErrFeedNotFound,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDatabaseError(tt.err)
			if got != tt.want {
				t.Errorf("IsDatabaseError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "rate limit error is retryable",
			err:  ErrRateLimitExceeded,
			want: true,
		},
		{
			name: "timeout error is retryable",
			err:  ErrOperationTimeout,
			want: true,
		},
		{
			name: "external service error is retryable",
			err:  ErrExternalServiceUnavailable,
			want: true,
		},
		{
			name: "feed not found is not retryable",
			err:  ErrFeedNotFound,
			want: false,
		},
		{
			name: "database unavailable is not retryable",
			err:  ErrDatabaseUnavailable,
			want: false,
		},
		{
			name: "validation error is not retryable",
			err:  ErrInvalidInput,
			want: false,
		},
		{
			name: "wrapped retryable error",
			err:  NewRateLimitExceededError("gateway", "APIGateway", "FetchData", nil, map[string]interface{}{"host": "api.example.com"}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryableError(tt.err)
			if got != tt.want {
				t.Errorf("IsRetryableError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewFeedNotFoundError(t *testing.T) {
	context := map[string]interface{}{
		"feed_id": 123,
		"url":     "https://example.com/feed",
	}

	err := NewFeedNotFoundError("gateway", "FeedGateway", "GetFeed", context)

	// Test error message format
	expectedMsg := "[gateway:FeedGateway:GetFeed] FEED_NOT_FOUND: feed not found (caused by: feed not found)"
	if err.Error() != expectedMsg {
		t.Errorf("NewFeedNotFoundError().Error() = %v, want %v", err.Error(), expectedMsg)
	}

	// Test that it wraps the sentinel error
	if !errors.Is(err, ErrFeedNotFound) {
		t.Error("NewFeedNotFoundError() should wrap ErrFeedNotFound")
	}

	// Test context preservation
	if err.Context["feed_id"] != 123 {
		t.Error("NewFeedNotFoundError() should preserve context")
	}
}

func TestNewDatabaseUnavailableError(t *testing.T) {
	cause := errors.New("connection timeout")
	context := map[string]interface{}{
		"host": "localhost",
		"port": 5432,
	}

	err := NewDatabaseUnavailableError("gateway", "DatabaseGateway", "Connect", cause, context)

	// Test that it wraps the sentinel error
	if !errors.Is(err, ErrDatabaseUnavailable) {
		t.Error("NewDatabaseUnavailableError() should wrap ErrDatabaseUnavailable")
	}

	// Test that it preserves the original cause
	if !errors.Is(err, cause) {
		t.Error("NewDatabaseUnavailableError() should preserve original cause in error chain")
	}
}

func TestErrorChainUnwrapping(t *testing.T) {
	originalCause := errors.New("connection reset")

	// Create a wrapped error chain
	dbErr := NewDatabaseUnavailableError("driver", "PostgresDriver", "Query", originalCause, map[string]interface{}{
		"query": "SELECT * FROM feeds",
	})

	// Test that we can unwrap to both the sentinel error and original cause
	if !errors.Is(dbErr, ErrDatabaseUnavailable) {
		t.Error("Error chain should contain ErrDatabaseUnavailable")
	}

	if !errors.Is(dbErr, originalCause) {
		t.Error("Error chain should contain original cause")
	}

	// Test using errors.As to extract AppContextError
	var appContextErr *AppContextError
	if !errors.As(dbErr, &appContextErr) {
		t.Error("Should be able to extract AppContextError from error chain")
	}

	if appContextErr.Layer != "driver" {
		t.Errorf("AppContextError.Layer = %v, want %v", appContextErr.Layer, "driver")
	}
}
