// Package feed_url_normalize provides canonical normalization for feed URLs
// used by GetFeedID lookups. The 2026-05-26 incident showed a 262-line burst
// of "feed not found" errors against a Reddit RSS URL whose stored variant
// differed from the requested one only by trailing slash / case / scheme.
// Normalising at the lookup boundary (and, when applied at register time
// also, at the DB boundary) makes the comparison resilient to those
// purely-presentational variants without losing semantic precision.
//
// Pure function. Idempotent: Normalize(Normalize(s)) == Normalize(s).
package feed_url_normalize

import (
	"net/url"
	"strings"
)

// Normalize returns a canonical form of feedURL. The transformation is
// conservative — it never rewrites the path semantically, only strips
// presentational noise that we are confident does not change identity:
//
//   - scheme lower-cased ("HTTPS://x" → "https://x")
//   - host lower-cased ("Example.COM" → "example.com")
//   - trailing dot stripped from host ("host." → "host")
//   - trailing slash stripped when path is non-empty and not the root ("/")
//   - default port stripped (":80" for http, ":443" for https)
//
// Inputs that fail to parse are returned unchanged so the caller can still
// perform a literal match. This is the safe default — a parse failure is
// usually a smell elsewhere and rewriting it would hide the upstream bug.
func Normalize(feedURL string) string {
	trimmed := strings.TrimSpace(feedURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return feedURL
	}
	// Scheme + host lower-case.
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Host)
	host = strings.TrimSuffix(host, ".")
	parsed.Host = host
	// Default-port strip.
	parsed.Host = stripDefaultPort(parsed.Scheme, parsed.Host)
	// Path trailing slash strip — only when path is more than just "/".
	if len(parsed.Path) > 1 && strings.HasSuffix(parsed.Path, "/") {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
	}
	return parsed.String()
}

func stripDefaultPort(scheme, host string) string {
	switch scheme {
	case "http":
		return strings.TrimSuffix(host, ":80")
	case "https":
		return strings.TrimSuffix(host, ":443")
	default:
		return host
	}
}
