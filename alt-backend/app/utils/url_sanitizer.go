package utils

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// trackingParams contains query parameter names to strip (lowercase for case-insensitive matching).
var trackingParams = map[string]bool{
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"utm_id":       true,
	"fbclid":       true,
	"gclid":        true,
	"mc_eid":       true,
	"msclkid":      true,
}

// StripTrackingParams removes known tracking parameters from a URL,
// preserving legitimate query parameters. Fragments are removed and
// remaining parameters are sorted by key for deterministic output.
func StripTrackingParams(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}
	if parsed.Scheme == "" {
		return "", fmt.Errorf("parse URL: missing scheme")
	}

	// Remove fragment
	parsed.Fragment = ""

	// Filter out tracking parameters (case-insensitive)
	query := parsed.Query()
	for param := range query {
		if trackingParams[strings.ToLower(param)] {
			query.Del(param)
		}
	}

	// Sort remaining parameters by key for deterministic output
	if len(query) > 0 {
		keys := make([]string, 0, len(query))
		for k := range query {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var params []string
		for _, k := range keys {
			for _, v := range query[k] {
				params = append(params, url.QueryEscape(k)+"="+url.QueryEscape(v))
			}
		}
		parsed.RawQuery = strings.Join(params, "&")
	} else {
		parsed.RawQuery = ""
	}

	result := parsed.String()

	// Remove trailing slash (except for scheme://)
	if len(result) > 1 && strings.HasSuffix(result, "/") && !strings.HasSuffix(result, "://") {
		result = strings.TrimRight(result, "/")
	}

	return result, nil
}
