package fetch_feed_gateway

import (
	"alt/utils/errors"
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSingleFeedGateway_EnhancedErrorHandling(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *SingleFeedGateway
		wantErr string
		checkError func(t *testing.T, err error)
	}{
		{
			name: "database unavailable returns AppContextError",
			setup: func() *SingleFeedGateway {
				// Create gateway with nil database to simulate unavailability
				return &SingleFeedGateway{
					alt_db:      nil,
					rateLimiter: nil,
				}
			},
			wantErr: "DATABASE_ERROR",
			checkError: func(t *testing.T, err error) {
				// Check that it's a database error using sentinel error pattern
				if !errors.IsDatabaseError(err) {
					t.Error("Expected database error to be detectable with IsDatabaseError()")
				}

				// Check that we can extract AppContextError
				var appContextErr *errors.AppContextError
				if !stdErrors.As(err, &appContextErr) {
					t.Error("Expected error to be extractable as AppContextError")
				} else {
					if appContextErr.Layer != "gateway" {
						t.Errorf("Expected layer to be 'gateway', got %s", appContextErr.Layer)
					}
					if appContextErr.Component != "SingleFeedGateway" {
						t.Errorf("Expected component to be 'SingleFeedGateway', got %s", appContextErr.Component)
					}
					if appContextErr.Operation != "FetchSingleFeed" {
						t.Errorf("Expected operation to be 'FetchSingleFeed', got %s", appContextErr.Operation)
					}
				}

				// Check HTTP status code mapping
				if appContextErr.HTTPStatusCode() != 500 {
					t.Errorf("Expected HTTP status 500, got %d", appContextErr.HTTPStatusCode())
				}

				// Check retryability
				if errors.IsRetryableError(err) {
					t.Error("Database errors should not be retryable")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := tt.setup()
			ctx := context.Background()

			_, err := gateway.FetchSingleFeed(ctx)

			if err == nil {
				t.Error("Expected error but got none")
				return
			}

			if tt.checkError != nil {
				tt.checkError(t, err)
			}
		})
	}
}

// Add alias for standard errors package to avoid naming conflict
import stdErrors "errors"

func TestSingleFeedGateway_ErrorContextEnrichment(t *testing.T) {
	// Test that errors are properly enriched with context as they bubble up through layers
	gateway := &SingleFeedGateway{
		alt_db:      nil, // Simulate database unavailability
		rateLimiter: nil,
	}

	ctx := context.Background()
	_, err := gateway.FetchSingleFeed(ctx)

	if err == nil {
		t.Fatal("Expected error but got none")
	}

	// Extract AppContextError to check context enrichment
	var appContextErr *errors.AppContextError
	if !stdErrors.As(err, &appContextErr) {
		t.Fatal("Expected error to be AppContextError")
	}

	// Check that context contains gateway-specific information
	if appContextErr.Context["component"] != "SingleFeedGateway" {
		t.Error("Expected context to contain gateway component information")
	}

	// Check that error chain preserves sentinel error
	if !errors.IsDatabaseError(err) {
		t.Error("Expected error chain to preserve database error detection")
	}
}

func TestSingleFeedGateway_RateLimitErrorHandling(t *testing.T) {
	// This test would require a mock rate limiter, but demonstrates the pattern
	// for testing rate limit error enrichment
	t.Skip("Requires mock rate limiter implementation")

	// The test would verify:
	// 1. Rate limit errors are properly wrapped with sentinel errors
	// 2. AppContextError contains rate limiting context
	// 3. IsRetryableError returns true for rate limit errors
	// 4. HTTP status code is 429 (Too Many Requests)
}

func TestSingleFeedGateway_ExternalAPIErrorHandling(t *testing.T) {
	// This test would require a mock HTTP server, but demonstrates the pattern
	// for testing external API error enrichment
	t.Skip("Requires mock HTTP server implementation")

	// The test would verify:
	// 1. External API errors are properly wrapped with sentinel errors
	// 2. AppContextError contains API context (URL, status code, etc.)
	// 3. IsRetryableError returns true for external service errors
	// 4. HTTP status code is 502 (Bad Gateway)
}

func TestErrorContextPreservation(t *testing.T) {
	// Test that when errors bubble up through layers, context is preserved and enriched
	ctx := context.Background()

	// Create an original database error from the driver layer
	originalErr := errors.NewDatabaseUnavailableError(
		"driver",
		"PostgresDriver", 
		"Connect",
		stdErrors.New("connection timeout"),
		map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
	)

	// Simulate gateway layer enriching the error
	enrichedErr := errors.EnrichWithContext(
		originalErr,
		"gateway",
		"SingleFeedGateway",
		"FetchSingleFeed",
		map[string]interface{}{
			"operation": "database_check",
			"timestamp": time.Now().Unix(),
		},
	)

	// Verify context preservation and enrichment
	if enrichedErr.Layer != "gateway" {
		t.Errorf("Expected enriched layer to be 'gateway', got %s", enrichedErr.Layer)
	}

	// Check that original context is preserved
	if enrichedErr.Context["host"] != "localhost" {
		t.Error("Expected original driver context to be preserved")
	}

	// Check that new context is added
	if enrichedErr.Context["operation"] != "database_check" {
		t.Error("Expected new gateway context to be added")
	}

	// Check that sentinel error detection still works
	if !errors.IsDatabaseError(enrichedErr) {
		t.Error("Expected sentinel error detection to work after enrichment")
	}
}