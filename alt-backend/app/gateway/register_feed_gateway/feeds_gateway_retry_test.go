package register_feed_gateway

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TDD Red Phase: Test retry mechanism with exponential backoff
func TestRegisterFeedGateway_RetryMechanism(t *testing.T) {
	gateway := &RegisterFeedGateway{
		alt_db: nil,
	}

	tests := []struct {
		name          string
		url           string
		expectedError string
		wantErr       bool
	}{
		{
			name:          "transient network error should trigger retry",
			url:           "https://httpbin.org/status/502", // Returns HTTP 502
			expectedError: "timeout",
			wantErr:       true,
		},
		{
			name:          "timeout error should trigger retry",
			url:           "https://httpbin.org/delay/5", // 5 second delay
			expectedError: "timeout",
			wantErr:       true, // Should fail after retries with short timeout
		},
		{
			name:          "non-retryable error should not retry",
			url:           "invalid-url", // Malformed URL
			expectedError: "URL must include a scheme",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := gateway.RegisterRSSFeedLink(ctx, tt.url)

			if !tt.wantErr && err != nil {
				t.Errorf("RegisterRSSFeedLink() unexpected error = %v", err)
				return
			}

			if tt.wantErr && err == nil {
				t.Errorf("RegisterRSSFeedLink() expected error, got nil")
				return
			}

			if tt.expectedError != "" && !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("RegisterRSSFeedLink() error = %v, want error containing %v", err, tt.expectedError)
			}
		})
	}
}
