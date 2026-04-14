package server

import "strings"

// restAllowlist enumerates the /v1/* paths that legitimately stay on plaintext
// REST after the user-facing migration to Connect-RPC (ADR-000729).
//
// Architectural justifications for each entry:
//   - /v1/images/proxy/       — browser <img src>, unauthenticated HMAC-signed
//   - /v1/images/fetch        — used by internal image flow, binary bytes
//   - /v1/dashboard/          — admin-only, low-traffic, REST ergonomic
//   - /v1/admin/scraping-domains — admin REST config, JWT-gated
//   - /v1/rss-feed-link/export/opml / import/opml — XML / multipart
//   - /v1/csrf-token          — security infra, single-shot
//   - /v1/health              — liveness probe
//
// Paths starting with any of these prefixes are forwarded to the plaintext
// alt-backend REST listener. Every other /v1/* path is rejected at the BFF
// boundary so accidental plaintext regressions surface immediately.
var restAllowlistPrefixes = []string{
	"/v1/images/proxy/",
	"/v1/images/fetch",
	"/v1/dashboard/",
	"/v1/admin/scraping-domains",
	"/v1/rss-feed-link/export/opml",
	"/v1/rss-feed-link/import/opml",
	"/v1/csrf-token",
	"/v1/health",
}

// allowRESTPath reports whether path is an approved REST-only endpoint.
func allowRESTPath(path string) bool {
	for _, prefix := range restAllowlistPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
