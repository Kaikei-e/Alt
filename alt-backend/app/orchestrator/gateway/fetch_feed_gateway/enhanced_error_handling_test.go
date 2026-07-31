package fetch_feed_gateway

import (
	"alt/domain"

	stderrors "errors"

	"alt/utils/errors"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
	"context"
	"testing"
	"time"
	// Add alias for standard errors package to avoid naming conflict
	stdErrors "errors"

	"github.com/google/uuid"
)

func TestSingleFeedGateway_EnhancedErrorHandling(t *testing.T) {
	// Initialize logger for testing to prevent nil pointer dereference
	logger.InitLogger()

	tests := []struct {
		name       string
		setup      func() *SingleFeedGateway
		wantErr    string
		checkError func(t *testing.T, err error)
	}{
		{
			name: "database unavailable returns AppContextError",
			setup: func() *SingleFeedGateway {
				// Create gateway with nil database to simulate unavailability
				return &SingleFeedGateway{
					store:       &pollableFeedLinkStoreStub{err: stderrors.New("data plane unavailable")},
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
					// The operation names the call that failed, which since
					// ADR-000954 Wave 3 batch 3 is the data plane read rather
					// than the enclosing gateway method. Asserting the
					// enclosing name would have hidden which of the two steps
					// broke.
					if appContextErr.Operation != "FetchRSSFeedURLs" {
						t.Errorf("Expected operation to be 'FetchRSSFeedURLs', got %s", appContextErr.Operation)
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

func TestSingleFeedGateway_ErrorContextEnrichment(t *testing.T) {
	// Initialize logger for testing to prevent nil pointer dereference
	logger.InitLogger()

	// Test that errors are properly enriched with context as they bubble up through layers
	gateway := &SingleFeedGateway{
		store:       &pollableFeedLinkStoreStub{err: stderrors.New("data plane unavailable")}, // Simulate database unavailability
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
	// Initialize logger for testing to prevent nil pointer dereference
	logger.InitLogger()

	// A real HostRateLimiter, pre-exhausted for this host so the call made
	// inside FetchSingleFeed is guaranteed to need to wait:
	//   1. warm-up call consumes the limiter's initial burst token.
	//   2. RecordRateLimitHit installs a fresh limiter with a long backoff -
	//      note it also starts with a full burst, so it takes one more
	//      immediate call to actually exhaust it.
	// The gateway's own call is then given a context with only a short
	// deadline left, far shorter than the 10s backoff, so rate.Limiter's
	// reservation check ("would exceed context deadline") fails immediately
	// and deterministically - it never actually sleeps. Crucially the feed
	// link fetch (which runs first, on the same context) is a stub and returns
	// instantly, leaving an enormous margin before the deadline - unlike a
	// pre-cancelled context, which would race that step too.
	const feedURL = "https://example.com/feed.xml"
	const feedHost = "example.com"
	limiter := rate_limiter.NewHostRateLimiter(time.Hour)
	if err := limiter.WaitForHost(context.Background(), feedURL); err != nil {
		t.Fatalf("warm-up call 1 failed: %v", err)
	}
	limiter.RecordRateLimitHit(feedHost, 10*time.Second)
	if err := limiter.WaitForHost(context.Background(), feedURL); err != nil {
		t.Fatalf("warm-up call 2 failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	gateway := &SingleFeedGateway{
		store:       &pollableFeedLinkStoreStub{links: []domain.FeedLink{{ID: uuid.New(), URL: "https://example.com/feed.xml"}}},
		rateLimiter: limiter,
	}

	_, err := gateway.FetchSingleFeed(ctx)
	if err == nil {
		t.Fatal("Expected rate limit error but got none")
	}

	// 1. Rate limit errors are properly wrapped with sentinel errors
	if !errors.IsRateLimitError(err) {
		t.Error("Expected error to be detectable with IsRateLimitError()")
	}

	// 2. AppContextError contains rate limiting context
	var appContextErr *errors.AppContextError
	if !stdErrors.As(err, &appContextErr) {
		t.Fatal("Expected error to be extractable as AppContextError")
	}
	if appContextErr.Context["operation"] != "rate_limit_wait" {
		t.Errorf("Expected context operation to be 'rate_limit_wait', got %v", appContextErr.Context["operation"])
	}
	if appContextErr.Context["url"] != "https://example.com/feed.xml" {
		t.Errorf("Expected context url to be the feed URL, got %v", appContextErr.Context["url"])
	}

	// 3. IsRetryableError returns true for rate limit errors
	if !errors.IsRetryableError(err) {
		t.Error("Expected rate limit errors to be retryable")
	}

	// 4. HTTP status code is 429 (Too Many Requests)
	if appContextErr.HTTPStatusCode() != 429 {
		t.Errorf("Expected HTTP status 429, got %d", appContextErr.HTTPStatusCode())
	}

}

func TestSingleFeedGateway_ExternalAPIErrorHandling(t *testing.T) {
	// Initialize logger for testing to prevent nil pointer dereference
	logger.InitLogger()

	// A URL that resolves but has nothing listening on it: gofeed's parse
	// will fail with a network/connection error, exercising the same
	// external-service error path a genuinely unreachable feed host would.

	gateway := &SingleFeedGateway{
		store:       &pollableFeedLinkStoreStub{links: []domain.FeedLink{{ID: uuid.New(), URL: "http://127.0.0.1:1/feed.xml"}}},
		rateLimiter: nil,
	}

	_, err := gateway.FetchSingleFeed(context.Background())
	if err == nil {
		t.Fatal("Expected external API error but got none")
	}

	// 1. External API errors are properly wrapped with sentinel errors
	if !errors.IsExternalServiceError(err) {
		t.Error("Expected error to be detectable with IsExternalServiceError()")
	}

	// 2. AppContextError contains API context (URL, parser, etc.)
	var appContextErr *errors.AppContextError
	if !stdErrors.As(err, &appContextErr) {
		t.Fatal("Expected error to be extractable as AppContextError")
	}
	if appContextErr.Context["url"] != "http://127.0.0.1:1/feed.xml" {
		t.Errorf("Expected context url to be the feed URL, got %v", appContextErr.Context["url"])
	}
	if appContextErr.Context["parser"] != "gofeed" {
		t.Errorf("Expected context parser to be 'gofeed', got %v", appContextErr.Context["parser"])
	}

	// 3. IsRetryableError returns true for external service errors
	if !errors.IsRetryableError(err) {
		t.Error("Expected external service errors to be retryable")
	}

	// 4. HTTP status code is 502 (Bad Gateway)
	if appContextErr.HTTPStatusCode() != 502 {
		t.Errorf("Expected HTTP status 502, got %d", appContextErr.HTTPStatusCode())
	}

}

func TestErrorContextPreservation(t *testing.T) {
	// Initialize logger for testing to prevent nil pointer dereference
	logger.InitLogger()

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
