package server

import "strings"

// restAllowlist enumerates the /v1/* paths that legitimately stay on plaintext
// REST after the user-facing migration to Connect-RPC (ADR-000729). It gates
// routing: server.go answers 404 for anything it refuses, so an omission here
// is a production outage, not a missing log line.
//
// Architectural justifications for each entry:
//   - /v1/images/proxy/       — browser <img src>, unauthenticated HMAC-signed.
//     nginx sends these straight to alt-backend today; the entry keeps the BFF
//     a working path rather than a second place to edit if that changes.
//   - /v1/images/fetch        — used by internal image flow, binary bytes
//   - /v1/dashboard/          — admin-only, low-traffic, REST ergonomic.
//     Covers metrics / overview / logs / jobs / recap_jobs.
//   - /v1/admin/scraping-domains — admin REST config, JWT-gated
//   - /v1/rss-feed-link/export/opml / import/opml — XML / multipart, and two
//     segments deep, so they need their own entries next to the
//     single-segment pattern below
//   - /v1/csrf-token          — security infra, single-shot
//   - /v1/health              — liveness probe
//   - /v1/feeds/read          — POST mark-as-read, still REST from the
//     SvelteKit /api/v1/feeds/read route
//   - /v1/feeds/stats/trends  — trend windows for the stats page
//   - /v1/articles/by-tag     — Tag Trail article listing
var restAllowlistPrefixes = []string{
	"/v1/images/proxy/",
	"/v1/images/fetch",
	"/v1/dashboard/",
	"/v1/admin/scraping-domains",
	"/v1/rss-feed-link/export/opml",
	"/v1/rss-feed-link/import/opml",
	"/v1/csrf-token",
	"/v1/health",
	"/v1/feeds/read",
	"/v1/feeds/stats/trends",
	"/v1/articles/by-tag",
}

// restAllowlistPatterns hold the endpoints whose path carries a resource id.
// `*` matches exactly one non-empty segment and the segment count must match,
// so these cannot be flattened into prefixes: "/v1/feeds/" as a prefix would
// re-open every migrated feeds endpoint, which is the opposite of enforcement.
//
// Architectural justifications for each entry:
//   - /v1/rss-feed-link/*  — the group's whole single-segment surface on
//     alt-backend is list / register / random / DELETE :id. The id is a free
//     UUID, so it is indistinguishable from the three verbs; one wildcard is
//     the honest expression and admits nothing the Echo group does not route.
//   - /v1/feeds/*/tags     — GET feed tags by feed id
//   - /v1/articles/*/tags  — GET article tags by article id
var restAllowlistPatterns = []string{
	"/v1/rss-feed-link/*",
	"/v1/feeds/*/tags",
	"/v1/articles/*/tags",
}

// allowRESTPath reports whether path is an approved REST-only endpoint.
func allowRESTPath(path string) bool {
	for _, prefix := range restAllowlistPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	for _, pattern := range restAllowlistPatterns {
		if matchRESTPattern(pattern, path) {
			return true
		}
	}
	return false
}

// matchRESTPattern reports whether path matches pattern segment by segment,
// where `*` stands for exactly one non-empty segment.
func matchRESTPattern(pattern, path string) bool {
	patternSegments := strings.Split(pattern, "/")
	pathSegments := strings.Split(path, "/")
	if len(patternSegments) != len(pathSegments) {
		return false
	}
	for i, want := range patternSegments {
		got := pathSegments[i]
		if want == "*" {
			if got == "" {
				return false
			}
			continue
		}
		if want != got {
			return false
		}
	}
	return true
}
