package register_feed_gateway

import (
	"testing"
)

// Security integration tests have been moved to validate_fetch_rss_gateway package,
// since URL validation + RSS fetching is now the responsibility of ValidateAndFetchRSSGateway.

func TestExtractSuggestedURLFromCertError(t *testing.T) {
	tests := []struct {
		name        string
		errStr      string
		originalURL string
		expected    string
	}{
		{
			name:        "extracts first valid domain",
			errStr:      "x509: certificate is valid for aar.art-it.asia, www.art-it.asia, not art-it.asia",
			originalURL: "https://art-it.asia/feed.xml",
			expected:    "https://aar.art-it.asia/feed.xml",
		},
		{
			name:        "returns empty when no match",
			errStr:      "some other error",
			originalURL: "https://example.com/feed.xml",
			expected:    "",
		},
		{
			name:        "preserves scheme and path",
			errStr:      "x509: certificate is valid for www.example.com, not example.com",
			originalURL: "https://example.com/rss/feed.xml",
			expected:    "https://www.example.com/rss/feed.xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSuggestedURLFromCertError(tt.errStr, tt.originalURL)
			if result != tt.expected {
				t.Errorf("extractSuggestedURLFromCertError() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBuildTLSErrorMessage(t *testing.T) {
	tests := []struct {
		name         string
		suggestedURL string
		expected     string
	}{
		{
			name:         "with suggested URL",
			suggestedURL: "https://www.example.com/feed.xml",
			expected:     "このURLの証明書に問題があります。https://www.example.com/feed.xml を試してください",
		},
		{
			name:         "without suggested URL",
			suggestedURL: "",
			expected:     "このURLの証明書に問題があります。別のURLを試してください",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTLSErrorMessage(tt.suggestedURL)
			if result != tt.expected {
				t.Errorf("buildTLSErrorMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}
