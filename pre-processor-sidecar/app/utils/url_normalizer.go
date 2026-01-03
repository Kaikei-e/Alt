// Package utils provides utility functions for the pre-processor-sidecar service
package utils

import (
	"net/url"
	"strings"
)

// trackingParams contains query parameters to remove during normalization
var trackingParams = map[string]bool{
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"utm_id":       true,
	"fbclid":       true, // Facebook click ID
	"gclid":        true, // Google click ID
	"mc_eid":       true, // MailChimp email ID
	"msclkid":      true, // Microsoft click ID
}

// NormalizeURL normalizes a URL by:
// - Removing tracking parameters (UTM, fbclid, etc.)
// - Removing URL fragments
// - Removing trailing slashes (except for root path)
func NormalizeURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Remove fragment
	parsed.Fragment = ""

	// Filter out tracking parameters
	query := parsed.Query()
	for param := range trackingParams {
		query.Del(param)
	}
	parsed.RawQuery = query.Encode()

	// Remove trailing slash (except for root path)
	if parsed.Path != "/" && strings.HasSuffix(parsed.Path, "/") {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
	}

	return parsed.String(), nil
}
