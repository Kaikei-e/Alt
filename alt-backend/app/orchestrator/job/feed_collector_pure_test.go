package job

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculate403Backoff(t *testing.T) {
	tests := []struct {
		name        string
		attempt     int
		retryAfter  time.Duration
		hasLimiter  bool
		wantBackoff time.Duration
	}{
		{
			name:        "without limiter attempt 1 uses 5s floor",
			attempt:     1,
			retryAfter:  0,
			hasLimiter:  false,
			wantBackoff: 5 * time.Second,
		},
		{
			name:        "without limiter attempt 2 doubles to 10s",
			attempt:     2,
			retryAfter:  0,
			hasLimiter:  false,
			wantBackoff: 10 * time.Second,
		},
		{
			name:        "without limiter attempt 3 doubles to 20s",
			attempt:     3,
			retryAfter:  0,
			hasLimiter:  false,
			wantBackoff: 20 * time.Second,
		},
		{
			name:        "with limiter attempt 1 uses 1s base",
			attempt:     1,
			retryAfter:  10 * time.Second,
			hasLimiter:  true,
			wantBackoff: 1 * time.Second,
		},
		{
			name:        "with limiter attempt 2 uses 2s",
			attempt:     2,
			retryAfter:  10 * time.Second,
			hasLimiter:  true,
			wantBackoff: 2 * time.Second,
		},
		{
			name:        "with limiter attempt 3 uses 4s",
			attempt:     3,
			retryAfter:  10 * time.Second,
			hasLimiter:  true,
			wantBackoff: 4 * time.Second,
		},
		{
			name:        "with limiter clamped to retryAfter when ladder exceeds interval",
			attempt:     3,
			retryAfter:  2 * time.Second,
			hasLimiter:  true,
			wantBackoff: 2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculate403Backoff(tt.attempt, tt.retryAfter, tt.hasLimiter)
			assert.Equal(t, tt.wantBackoff, got)
		})
	}
}

func TestIs403Error_Pure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error returns false",
			err:  nil,
			want: false,
		},
		{
			name: "error containing 403 returns true",
			err:  errors.New("HTTP response status: 403 Forbidden"),
			want: true,
		},
		{
			name: "error without 403 returns false",
			err:  errors.New("HTTP response status: 500 Internal Server Error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := is403Error(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateFeedURL_Pure(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{
			name:    "valid http URL",
			rawURL:  "http://example.com/rss",
			wantErr: false,
		},
		{
			name:    "valid https URL",
			rawURL:  "https://example.com/feed.xml",
			wantErr: false,
		},
		{
			name:    "invalid ftp scheme",
			rawURL:  "ftp://example.com/feed.xml",
			wantErr: true,
		},
		{
			name:    "missing scheme",
			rawURL:  "//example.com/feed.xml",
			wantErr: true,
		},
		{
			name:    "missing host",
			rawURL:  "http:///feed.xml",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := url.Parse(tt.rawURL)
			assert.NoError(t, err)
			validateErr := validateFeedURL(*parsed)
			if tt.wantErr {
				assert.Error(t, validateErr)
			} else {
				assert.NoError(t, validateErr)
			}
		})
	}
}
