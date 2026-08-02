package server

import "testing"

// allowRESTPath is the gate on the "/v1/" route: server.go rejects anything it
// refuses with 404. The table below is therefore the contract for what stays
// reachable over plaintext REST — every `true` row is a live frontend caller,
// every `false` row is either migrated to Connect-RPC or was never a
// browser-facing endpoint.
func TestRESTAllowlist_Matches(t *testing.T) {
	cases := []struct {
		path  string
		allow bool
	}{
		// allowed — prefixes
		{"/v1/images/proxy/abc/https%3A%2F%2Fexample.com%2Fx.jpg", true},
		{"/v1/images/fetch", true},
		{"/v1/dashboard/metrics", true},
		{"/v1/dashboard/jobs", true},
		{"/v1/dashboard/recap_jobs", true},
		{"/v1/admin/scraping-domains", true},
		{"/v1/admin/scraping-domains/example.com", true},
		{"/v1/rss-feed-link/export/opml", true},
		{"/v1/rss-feed-link/import/opml", true},
		{"/v1/csrf-token", true},
		{"/v1/health", true},
		{"/v1/feeds/read", true},
		{"/v1/feeds/stats/trends", true},
		{"/v1/articles/by-tag", true},

		// allowed — single-segment patterns
		{"/v1/rss-feed-link/list", true},
		{"/v1/rss-feed-link/register", true},
		{"/v1/rss-feed-link/random", true},
		{"/v1/rss-feed-link/0f8fad5b-d9cb-469f-a165-70867728950e", true},
		{"/v1/feeds/0f8fad5b-d9cb-469f-a165-70867728950e/tags", true},
		{"/v1/articles/123/tags", true},

		// a wildcard matches exactly one non-empty segment
		{"/v1/feeds/x/y/tags", false},
		{"/v1/feeds//tags", false},
		{"/v1/articles//tags", false},
		{"/v1/articles/123/tags/extra", false},
		{"/v1/rss-feed-link/", false},
		{"/v1/rss-feed-link/a/b", false},

		// rejected — these have Connect-RPC equivalents or were migrated away
		{"/v1/feeds/fetch/cursor", false},
		{"/v1/feeds/register/favorite", false},
		{"/v1/feeds/tags", false},
		{"/v1/feeds/stats", false},
		{"/v1/feeds/stats/detailed", false},
		{"/v1/feeds/count/unreads", false},
		{"/v1/articles/fetch/content", false},
		{"/v1/articles/search", false},
		{"/v1/morning-letter/updates", false},

		// no match at all → rejected
		{"/v1/unknown", false},
		{"/v1/", false},
		{"/v1/feeds", false},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			got := allowRESTPath(c.path)
			if got != c.allow {
				t.Errorf("allowRESTPath(%q) = %v, want %v", c.path, got, c.allow)
			}
		})
	}
}
