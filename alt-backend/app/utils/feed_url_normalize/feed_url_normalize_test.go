package feed_url_normalize

import "testing"

func TestNormalize_TableDriven(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		out  string
	}{
		// Identity for empty / already-canonical input.
		{"empty", "", ""},
		{"already canonical", "https://example.com/feed.rss", "https://example.com/feed.rss"},

		// The Reddit r/webdev incident shape — trailing slash stripped on
		// non-root paths so a stored variant without the slash matches a
		// lookup that has it (or vice versa).
		{"trailing slash on path", "https://www.reddit.com/r/webdev/.rss/", "https://www.reddit.com/r/webdev/.rss"},
		{"trailing slash on multi-segment path", "https://example.com/a/b/c/", "https://example.com/a/b/c"},
		{"root slash kept", "https://example.com/", "https://example.com/"},

		// Scheme & host case.
		{"upper scheme", "HTTPS://example.com/x", "https://example.com/x"},
		{"upper host", "https://Example.COM/x", "https://example.com/x"},
		{"mixed case + slash", "HTTPS://Example.COM/X/", "https://example.com/X"},

		// Trailing-dot host (rare but valid DNS form).
		{"trailing dot host", "https://example.com./feed", "https://example.com/feed"},

		// Default port elision.
		{"default https port", "https://example.com:443/feed", "https://example.com/feed"},
		{"default http port", "http://example.com:80/feed", "http://example.com/feed"},
		{"non-default port kept", "https://example.com:8443/feed", "https://example.com:8443/feed"},

		// Path semantics preserved — the function must NOT touch query / fragment.
		{"query preserved", "https://example.com/feed.rss?utm=1", "https://example.com/feed.rss?utm=1"},
		{"path case preserved", "https://example.com/Feed.RSS", "https://example.com/Feed.RSS"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := Normalize(c.in)
			if got != c.out {
				t.Errorf("Normalize(%q) = %q; want %q", c.in, got, c.out)
			}
		})
	}
}

// TestNormalize_Idempotent — applying Normalize twice yields the same string.
// This is a load-bearing invariant for any future caller that wants to compare
// stored URLs against incoming URLs without worrying about repeated passes.
func TestNormalize_Idempotent(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"https://www.reddit.com/r/webdev/.rss/",
		"HTTPS://Example.COM/A/B/",
		"http://example.com:80/feed/",
	}
	for _, in := range inputs {
		once := Normalize(in)
		twice := Normalize(once)
		if once != twice {
			t.Errorf("Normalize not idempotent for %q: %q vs %q", in, once, twice)
		}
	}
}

// TestNormalize_BadInputUnchanged — unparseable input falls through unchanged so
// the caller can still surface a literal match (or a real parsing error from a
// later layer) instead of getting a silently rewritten URL.
func TestNormalize_BadInputUnchanged(t *testing.T) {
	t.Parallel()
	// `://` with no scheme is not a parse error in net/url, but a control
	// byte is. We use ASCII NUL to force the parser to bail.
	bad := "https://example.com/\x00path"
	got := Normalize(bad)
	if got != bad {
		t.Errorf("expected bad input returned unchanged, got %q", got)
	}
}
