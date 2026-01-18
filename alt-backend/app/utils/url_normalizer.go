package utils

import (
	"net/url"
	"strings"
)

// NormalizeURL normalizes a URL by removing all query parameters, fragments, and trailing slashes.
// This ensures consistent URL comparison and storage for feed URLs.
//
// Modifications applied:
//   - Removes ALL query parameters (they are noise for feed URLs)
//   - Removes URL fragments (#anchor)
//   - Removes trailing slashes (including root path "/" for consistency)
//
// Example:
//
//	input:  "https://example.com/article?utm_source=rss&q=test#section"
//	output: "https://example.com/article"
func NormalizeURL(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Remove ALL query parameters (they are noise for feed URLs)
	parsedURL.RawQuery = ""

	// Remove fragment
	parsedURL.Fragment = ""

	// Remove trailing slash (including root path for consistency)
	if parsedURL.Path == "/" {
		parsedURL.Path = ""
	} else if strings.HasSuffix(parsedURL.Path, "/") {
		parsedURL.Path = strings.TrimRight(parsedURL.Path, "/")
	}

	return parsedURL.String(), nil
}

// URLsEqual compares two normalized URLs with case-insensitive percent-encoding.
// This handles the issue where Go's url.Parse() may capitalize percent-encoding
// (e.g., %e3 becomes %E3), causing comparison failures with URLs stored in lowercase.
//
// Example:
//
//	url1: "https://example.com/path%e3%81%82"
//	url2: "https://example.com/path%E3%81%82"
//	result: true (they represent the same URL)
func URLsEqual(url1, url2 string) bool {
	// Convert both URLs to lowercase for case-insensitive comparison
	// This handles percent-encoding case differences (%e3 vs %E3)
	return strings.EqualFold(url1, url2)
}
