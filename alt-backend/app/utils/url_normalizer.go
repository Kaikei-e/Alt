package utils

import (
	"net/url"
	"strings"
)

// NormalizeURL normalizes a URL by removing tracking parameters, fragments, and trailing slashes.
// This ensures consistent URL comparison regardless of tracking parameters or formatting variations.
//
// Modifications applied:
//   - Removes common tracking parameters (UTM, fbclid, gclid, etc.)
//   - Removes URL fragments (#anchor)
//   - Removes trailing slashes (except for root path "/")
//
// Example:
//
//	input:  "https://example.com/article?utm_source=rss&utm_campaign=test/"
//	output: "https://example.com/article"
func NormalizeURL(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Remove common tracking parameters
	query := parsedURL.Query()
	trackingParams := []string{
		"utm_source", "utm_medium", "utm_campaign",
		"utm_term", "utm_content", "utm_id",
		"fbclid", "gclid", "mc_eid", "msclkid",
	}
	for _, param := range trackingParams {
		query.Del(param)
	}
	parsedURL.RawQuery = query.Encode()

	// Remove fragment
	parsedURL.Fragment = ""

	// Remove trailing slash (except for root path)
	if parsedURL.Path != "/" && strings.HasSuffix(parsedURL.Path, "/") {
		parsedURL.Path = strings.TrimRight(parsedURL.Path, "/")
	}

	return parsedURL.String(), nil
}
