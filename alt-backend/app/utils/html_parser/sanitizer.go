package html_parser

import (
	"strings"

	"codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
)

// SanitizeHTML strips unsafe tags and scripts but preserves structural HTML using bluemonday.
func SanitizeHTML(raw string) string {
	p := bluemonday.UGCPolicy()
	// Allow common structural elements that might contain content
	p.AllowElements("article", "section", "div", "p", "span", "br", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "blockquote", "pre", "code", "b", "strong", "i", "em", "u", "a", "img")
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("src", "alt", "title").OnElements("img")

	return p.Sanitize(raw)
}

// ExtractArticleHTML extracts main article content and returns sanitized HTML.
// Unlike ExtractArticleText which returns plain text, this preserves structural HTML
// (headers, lists, code blocks, images, links) while removing unsafe elements.
// It uses go-readability for content extraction and bluemonday for sanitization.
func ExtractArticleHTML(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	// Short-circuit if the payload is already plain text (no HTML tags).
	if !strings.Contains(trimmed, "<") {
		if len(trimmed) < MinArticleLength {
			return ""
		}
		return trimmed
	}

	// Pre-process: Remove non-content elements before go-readability
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err == nil {
		// Remove navigation, header, footer, aside
		doc.Find("head, script, style, noscript, title, aside, nav, header, footer").Remove()

		// Remove media and embedded content (ads, tracking, etc.)
		doc.Find("iframe, embed, object, video, audio, canvas, svg, math, form").Remove()

		// Remove social media elements
		doc.Find("[class*='social'], [class*='share'], [class*='twitter'], [class*='facebook'], [class*='instagram'], [class*='linkedin']").Remove()
		doc.Find("[id*='social'], [id*='share'], [id*='twitter'], [id*='facebook']").Remove()

		// Remove comment sections
		doc.Find("[class*='comment'], [id*='comment'], [class*='discussion'], [id*='discussion']").Remove()

		// Remove common non-content containers (menus, sidebars)
		doc.Find("[class*='menu'], [id*='menu'], [class*='sidebar'], [id*='sidebar'], [class*='widget'], [id*='widget']").Remove()
		doc.Find("[role='navigation'], [role='banner'], [role='contentinfo']").Remove()

		cleanedHTML, _ := doc.Html()
		if cleanedHTML != "" {
			trimmed = cleanedHTML
		}
	}

	// Use go-readability to extract main content
	article, err := readability.FromReader(strings.NewReader(trimmed), nil)
	if err == nil {
		var htmlBuf strings.Builder
		if err := article.RenderHTML(&htmlBuf); err == nil {
			html := strings.TrimSpace(htmlBuf.String())
			if html != "" {
				sanitized := sanitizeArticleHTML(html)
				if len(strings.TrimSpace(StripTags(sanitized))) >= MinArticleLength {
					return sanitized
				}
			}
		}
	}

	// Fallback: Sanitize the original HTML directly
	sanitized := sanitizeArticleHTML(trimmed)
	if len(strings.TrimSpace(StripTags(sanitized))) < MinArticleLength {
		return ""
	}
	return sanitized
}

// sanitizeArticleHTML sanitizes HTML using bluemonday while preserving rich content.
// It allows structural elements, text formatting, links, images, and tables
// while removing scripts, event handlers, and other potentially dangerous content.
func sanitizeArticleHTML(raw string) string {
	p := bluemonday.NewPolicy()

	// Structural elements
	p.AllowElements("article", "section", "div", "p", "span", "br")

	// Headers
	p.AllowElements("h1", "h2", "h3", "h4", "h5", "h6")

	// Lists
	p.AllowElements("ul", "ol", "li")

	// Quotes and code
	p.AllowElements("blockquote", "pre", "code")

	// Text formatting
	p.AllowElements("b", "strong", "i", "em", "u", "s", "del", "ins", "mark", "sub", "sup")

	// Links - allow http/https URLs and relative URLs
	p.AllowStandardURLs()
	p.AllowRelativeURLs(true)
	p.AllowAttrs("href").OnElements("a")
	p.RequireNoFollowOnLinks(false)
	p.RequireNoReferrerOnLinks(false)

	// NOTE: img tags are intentionally NOT allowed
	// - Alt doesn't fetch/display images anyway
	// - img tags are a major XSS vector (onerror, onload events)

	// Tables
	p.AllowElements("table", "thead", "tbody", "tfoot", "tr", "th", "td", "caption", "colgroup", "col")

	// Horizontal rule
	p.AllowElements("hr")

	// Figure and figcaption for images with captions
	p.AllowElements("figure", "figcaption")

	// Definition lists
	p.AllowElements("dl", "dt", "dd")

	// Only allow safe URL schemes (http, https, mailto) - blocks javascript: and data:
	p.AllowURLSchemes("http", "https", "mailto")

	return p.Sanitize(raw)
}
