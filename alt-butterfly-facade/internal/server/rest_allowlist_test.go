package server

import "testing"

// allowRESTPath is a pure classification function only; it does not itself
// enforce routing. See server.go's "/v1/" handler for how its result is
// currently used (warn-only logging, not rejection — ADR-000729 Phase 3 is
// not implemented yet) and server_test.go's
// TestServer_RESTRoute_NonAllowlistedPathIsStillForwarded for the routing
// behavior this implies today.
func TestRESTAllowlist_Matches(t *testing.T) {
	cases := []struct {
		path  string
		allow bool
	}{
		// allowed
		{"/v1/images/proxy/abc/https%3A%2F%2Fexample.com%2Fx.jpg", true},
		{"/v1/images/fetch", true},
		{"/v1/dashboard/metrics", true},
		{"/v1/dashboard/jobs", true},
		{"/v1/admin/scraping-domains", true},
		{"/v1/admin/scraping-domains/example.com", true},
		{"/v1/rss-feed-link/export/opml", true},
		{"/v1/rss-feed-link/import/opml", true},
		{"/v1/csrf-token", true},
		{"/v1/health", true},
		// rejected — these have Connect-RPC equivalents or were migrated away
		{"/v1/feeds/fetch/cursor", false},
		{"/v1/feeds/register/favorite", false},
		{"/v1/feeds/tags", false},
		{"/v1/feeds/xyz/tags", false},
		{"/v1/articles/by-tag", false},
		{"/v1/articles/abc/tags", false},
		{"/v1/rss-feed-link/list", false},
		{"/v1/rss-feed-link/random", false},
		{"/v1/morning-letter/updates", false},
		// no prefix match → rejected
		{"/v1/unknown", false},
		{"/v1/", false},
	}
	for _, c := range cases {
		got := allowRESTPath(c.path)
		if got != c.allow {
			t.Errorf("allowRESTPath(%q) = %v, want %v", c.path, got, c.allow)
		}
	}
}
