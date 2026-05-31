package html_parser

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ExtractHead extracts the raw <head> section from HTML.
func ExtractHead(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err != nil {
		return ""
	}

	headSel := doc.Find("head")
	if headSel.Length() == 0 {
		return ""
	}

	html, err := headSel.Html()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(html)
}

// ExtractOgImageURL extracts the best preview image URL from raw HTML and
// resolves it to an absolute http/https URL using baseURL.
//
// Candidate priority: og:image:secure_url > og:image:url > og:image >
// name=og:image > twitter:image > twitter:image:src > link[rel=image_src].
// Relative and protocol-relative URLs are resolved against baseURL; data: and
// other unsupported schemes are skipped in favour of the next candidate.
func ExtractOgImageURL(raw, baseURL string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err != nil {
		return ""
	}

	var base *url.URL
	if b := strings.TrimSpace(baseURL); b != "" {
		if parsed, perr := url.Parse(b); perr == nil {
			base = parsed
		}
	}

	for _, candidate := range ogImageCandidates(doc) {
		if resolved := resolveImageURL(candidate, base); resolved != "" {
			return resolved
		}
	}
	return ""
}

// ogImageCandidates returns the raw image URL candidates found in <head>,
// in descending order of preference.
func ogImageCandidates(doc *goquery.Document) []string {
	selectors := []struct {
		query string
		attr  string
	}{
		{`meta[property='og:image:secure_url']`, "content"},
		{`meta[property='og:image:url']`, "content"},
		{`meta[property='og:image']`, "content"},
		{`meta[name='og:image']`, "content"},
		{`meta[name='twitter:image']`, "content"},
		{`meta[name='twitter:image:src']`, "content"},
		{`link[rel='image_src']`, "href"},
	}

	candidates := make([]string, 0, len(selectors))
	for _, s := range selectors {
		if v, ok := doc.Find(s.query).First().Attr(s.attr); ok {
			if v = strings.TrimSpace(v); v != "" {
				candidates = append(candidates, v)
			}
		}
	}
	return candidates
}

// resolveImageURL returns an absolute http/https URL, or "" if the candidate is
// unusable (empty, a data:/javascript: scheme, or relative without a base).
func resolveImageURL(candidate string, base *url.URL) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}

	u, err := url.Parse(candidate)
	if err != nil {
		return ""
	}

	// Absolute http/https URL — accept as-is.
	if u.Scheme == "http" || u.Scheme == "https" {
		if u.Host == "" {
			return ""
		}
		return u.String()
	}

	// Any other explicit scheme (data:, javascript:, mailto:, ...) is unusable.
	if u.Scheme != "" {
		return ""
	}

	// Scheme-less reference (protocol-relative or relative path) — resolve it
	// against the base URL of the page it was found on.
	if base == nil {
		return ""
	}
	resolved := base.ResolveReference(u)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	if resolved.Host == "" {
		return ""
	}
	return resolved.String()
}
