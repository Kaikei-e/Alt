package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURLSecurityValidator_ValidateRSSURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid HTTPS RSS URL should pass",
			url:     "https://example.com/feed.xml",
			wantErr: false,
		},
		{
			name:    "valid HTTP RSS URL should pass",
			url:     "http://example.com/rss",
			wantErr: false,
		},
		{
			name:    "private IP should fail",
			url:     "http://192.168.1.1/feed",
			wantErr: true,
			errMsg:  "private network access denied",
		},
		{
			name:    "localhost should fail",
			url:     "http://localhost/feed",
			wantErr: true,
			errMsg:  "private network access denied",
		},
		{
			name:    "127.0.0.1 should fail",
			url:     "http://127.0.0.1/feed",
			wantErr: true,
			errMsg:  "private network access denied",
		},
		{
			name:    "10.0.0.0 network should fail",
			url:     "http://10.0.0.1/feed",
			wantErr: true,
			errMsg:  "private network access denied",
		},
		{
			name:    "172.16.0.0 network should fail",
			url:     "http://172.16.0.1/feed",
			wantErr: true,
			errMsg:  "private network access denied",
		},
		{
			name:    "non-HTTP scheme should fail",
			url:     "ftp://example.com/feed",
			wantErr: true,
			errMsg:  "only HTTP and HTTPS schemes allowed",
		},
		{
			name:    "javascript scheme should fail",
			url:     "javascript:alert('xss')",
			wantErr: true,
			errMsg:  "only HTTP and HTTPS schemes allowed",
		},
		{
			name:    "file scheme should fail",
			url:     "file:///etc/passwd",
			wantErr: true,
			errMsg:  "only HTTP and HTTPS schemes allowed",
		},
		{
			name:    "malformed URL should fail",
			url:     "not-a-url",
			wantErr: true,
			errMsg:  "only HTTP and HTTPS schemes allowed",
		},
		{
			name:    "empty URL should fail",
			url:     "",
			wantErr: true,
			errMsg:  "URL cannot be empty",
		},
		{
			name:    "URL with directory traversal should fail",
			url:     "http://example.com/../../../etc/passwd",
			wantErr: true,
			errMsg:  "URL contains dangerous pattern",
		},
		{
			name:    "URL with metadata server should fail",
			url:     "http://metadata.google.internal/",
			wantErr: true,
			errMsg:  "metadata server access denied",
		},
		{
			name:    "extremely long URL should fail",
			url:     "http://example.com/" + string(make([]byte, 3000)),
			wantErr: true,
			errMsg:  "URL exceeds maximum length",
		},
	}

	validator := NewURLSecurityValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateRSSURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err, "Expected error for URL: %s", tt.url)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg, "Error message should contain: %s", tt.errMsg)
				}
			} else {
				assert.NoError(t, err, "Expected no error for URL: %s", tt.url)
			}
		})
	}
}

func TestURLSecurityValidator_ValidateForRSSFeed(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "RSS feed URL should pass",
			url:     "https://example.com/feed.xml",
			wantErr: false,
		},
		{
			name:    "Atom feed URL should pass",
			url:     "https://example.com/atom.xml",
			wantErr: false,
		},
		{
			name:    "feed directory should pass",
			url:     "https://example.com/feeds/news",
			wantErr: false,
		},
		{
			name:    "non-feed path should fail",
			url:     "https://example.com/login",
			wantErr: true,
			errMsg:  "URL path does not appear to be an RSS feed",
		},
	}

	validator := NewURLSecurityValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateForRSSFeed(tt.url)
			if tt.wantErr {
				assert.Error(t, err, "Expected error for URL: %s", tt.url)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg, "Error message should contain: %s", tt.errMsg)
				}
			} else {
				assert.NoError(t, err, "Expected no error for URL: %s", tt.url)
			}
		})
	}
}

func TestURLSecurityValidator_IsAllowedDomain(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		expected bool
	}{
		{
			name:     "public domain should be allowed",
			domain:   "example.com",
			expected: true,
		},
		{
			name:     "localhost should not be allowed",
			domain:   "localhost",
			expected: false,
		},
		{
			name:     "private IP should not be allowed",
			domain:   "192.168.1.1",
			expected: false,
		},
		{
			name:     "metadata server should not be allowed",
			domain:   "metadata.google.internal",
			expected: false,
		},
	}

	validator := NewURLSecurityValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.IsAllowedDomain(tt.domain)
			assert.Equal(t, tt.expected, result, "Domain %s should be %v", tt.domain, tt.expected)
		})
	}
}
