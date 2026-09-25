package security

import (
	"net/url"
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
			name:    "0.0.0.0 should fail",
			url:     "http://0.0.0.0/feed",
			wantErr: true,
			errMsg:  "private network access denied",
		},
		{
			name:    "CGNAT 100.64.0.1 should fail",
			url:     "http://100.64.0.1/feed",
			wantErr: true,
			errMsg:  "private network access denied",
		},
		{
			name:    ":: unspecified IPv6 should fail",
			url:     "http://[::]/feed",
			wantErr: true,
			errMsg:  "private network access denied",
		},
		{
			name:    "multicast 224.0.0.1 should fail",
			url:     "http://224.0.0.1/feed",
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
				if assert.Error(t, err, "Expected error for URL: %s", tt.url) && tt.errMsg != "" {
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

func TestURLSecurityValidator_ValidateParsedRSSURL(t *testing.T) {
	v := NewURLSecurityValidator()

	t.Run("nil URL returns error", func(t *testing.T) {
		err := v.ValidateParsedRSSURL(nil)
		assert.EqualError(t, err, "nil URL")
	})

	t.Run("rejects userinfo", func(t *testing.T) {
		u, err := url.Parse("http://admin:secret@example.com/feed.xml")
		assert.NoError(t, err)
		err = v.ValidateParsedRSSURL(u)
		assert.EqualError(t, err, "userinfo not allowed in URL")
	})

	t.Run("port restrictions", func(t *testing.T) {
		// Disallowed ports
		for _, raw := range []string{
			"http://example.com:8080/feed.xml",
			"http://example.com:22/feed.xml",
			"https://example.com:8443/feed.xml",
		} {
			u, err := url.Parse(raw)
			assert.NoError(t, err)
			err = v.ValidateParsedRSSURL(u)
			assert.EqualError(t, err, "port not allowed")
		}

		// Allowed ports (empty, 80, 443) for public domain (mock via allowlist to avoid external DNS)
		t.Setenv("FEED_ALLOWED_HOSTS", "example.com")
		for _, raw := range []string{
			"http://example.com/feed.xml",
			"http://example.com:80/feed.xml",
			"https://example.com:443/feed.xml",
		} {
			u, err := url.Parse(raw)
			assert.NoError(t, err)
			assert.NoError(t, v.ValidateParsedRSSURL(u))
		}
	})

	t.Run("bare IP literals blocked", func(t *testing.T) {
		for _, raw := range []string{
			"http://0.0.0.0/feed.xml",
			"http://0.0.0.1/feed.xml",
			"http://[::]/feed.xml",
			"http://100.64.0.1/feed.xml",
			"http://100.64.0.1:80/feed.xml",
			"http://224.0.0.1/feed.xml",
			"http://240.0.0.1/feed.xml",
			"http://255.255.255.255/feed.xml",
			"http://198.18.0.1/feed.xml",
		} {
			u, err := url.Parse(raw)
			assert.NoError(t, err)
			assert.Error(t, v.ValidateParsedRSSURL(u))
		}
	})

	t.Run("disallowed scheme gopher", func(t *testing.T) {
		u, err := url.Parse("gopher://example.com")
		assert.NoError(t, err)
		err = v.ValidateParsedRSSURL(u)
		assert.EqualError(t, err, "only HTTP and HTTPS schemes allowed")
	})
}
