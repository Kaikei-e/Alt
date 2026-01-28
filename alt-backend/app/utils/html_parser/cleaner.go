package html_parser

import (
	"alt/domain"
	"alt/utils/constants"
	"encoding/json"
	"strings"

	"codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
)

// MinArticleLength is re-exported for backward compatibility
const MinArticleLength = constants.MinArticleLength

// SanitizeHTML strips unsafe tags and scripts but preserves structural HTML using bluemonday.
func SanitizeHTML(raw string) string {
	p := bluemonday.UGCPolicy()
	// Allow common structural elements that might contain content
	p.AllowElements("article", "section", "div", "p", "span", "br", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "blockquote", "pre", "code", "b", "strong", "i", "em", "u", "a", "img")
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("src", "alt", "title").OnElements("img")

	return p.Sanitize(raw)
}

// ExtractArticleText converts raw article HTML into plain text paragraphs.
// It uses multiple extraction strategies in order of priority:
// 1. Next.js __NEXT_DATA__ JSON extraction
// 2. go-readability content extraction
// 3. Fallback tag stripping
func ExtractArticleText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	// Short-circuit if the payload is already plain text.
	if !strings.Contains(trimmed, "<") {
		return checkLength(normalizeWhitespace(trimmed))
	}

	// Prepare goquery document for further inspection
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err == nil {
		// Strategy 1: Try Next.js __NEXT_DATA__ extraction (highest priority)
		if text := extractFromNextData(doc); text != "" {
			return checkLength(text)
		}

		// Pre-process: Remove non-content elements before go-readability
		removeNonContentElements(doc)

		cleanedHTML, _ := doc.Html()
		if cleanedHTML != "" {
			trimmed = cleanedHTML
		}
	}

	// Strategy 2: Try go-readability on the cleaned document
	if text := extractWithReadability(trimmed); text != "" {
		return checkLength(text)
	}

	// Strategy 3: Final fallback - strip tags from the original HTML
	return checkLength(extractParagraphs(trimmed))
}

// extractFromNextData attempts to extract article content from Next.js __NEXT_DATA__ script.
// Next.js sites often store the full article content in this JSON script.
func extractFromNextData(doc *goquery.Document) string {
	nextData := doc.Find("script[id='__NEXT_DATA__']")
	if nextData.Length() == 0 {
		return ""
	}

	jsonData := nextData.Text()
	var data map[string]any
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		return ""
	}

	// Traverse: props -> pageProps -> article -> bodyHtml
	props, ok := data["props"].(map[string]any)
	if !ok {
		return ""
	}
	pageProps, ok := props["pageProps"].(map[string]any)
	if !ok {
		return ""
	}
	articleData, ok := pageProps["article"].(map[string]any)
	if !ok {
		return ""
	}

	title, _ := articleData["title"].(string)
	bodyHtml, ok := articleData["bodyHtml"].(string)
	if !ok || len(bodyHtml) == 0 {
		return ""
	}

	text := extractParagraphs(bodyHtml)
	if len(text) == 0 {
		return ""
	}

	if title != "" {
		return title + "\n\n" + text
	}
	return text
}

// removeNonContentElements removes common non-content elements from the HTML document.
// This includes navigation, headers, footers, scripts, styles, social media widgets, etc.
func removeNonContentElements(doc *goquery.Document) {
	// Remove navigation, header, footer, aside
	doc.Find("head, script, style, noscript, title, aside, nav, header, footer").Remove()

	// Remove media and embedded content (ads, tracking, etc.)
	doc.Find("iframe, embed, object, video, audio, canvas").Remove()

	// Remove social media elements
	doc.Find("[class*='social'], [class*='share'], [class*='twitter'], [class*='facebook'], [class*='instagram'], [class*='linkedin']").Remove()
	doc.Find("[id*='social'], [id*='share'], [id*='twitter'], [id*='facebook']").Remove()

	// Remove comment sections
	doc.Find("[class*='comment'], [id*='comment'], [class*='discussion'], [id*='discussion']").Remove()

	// Remove common non-content containers (menus, sidebars)
	doc.Find("[class*='menu'], [id*='menu'], [class*='sidebar'], [id*='sidebar'], [class*='widget'], [id*='widget']").Remove()
	doc.Find("[role='navigation'], [role='banner'], [role='contentinfo']").Remove()

	// Remove metadata and resource links
	doc.Find("meta, link[rel='stylesheet'], link[rel='preload'], link[rel='prefetch'], link[rel='dns-prefetch']").Remove()

	// Remove inline styles and event handlers from all elements
	removeEventHandlers(doc)
}

// removeEventHandlers strips inline styles and event handler attributes from all elements.
func removeEventHandlers(doc *goquery.Document) {
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		s.RemoveAttr("style")
		s.RemoveAttr("onclick")
		s.RemoveAttr("onload")
		s.RemoveAttr("onerror")
		s.RemoveAttr("onmouseover")
		s.RemoveAttr("onmouseout")
		s.RemoveAttr("onfocus")
		s.RemoveAttr("onblur")
		s.RemoveAttr("onchange")
		s.RemoveAttr("onsubmit")
	})
}

// extractWithReadability uses go-readability to extract the main article content.
// Returns the extracted text, or empty string if extraction fails.
func extractWithReadability(html string) string {
	article, err := readability.FromReader(strings.NewReader(html), nil)
	if err != nil {
		return ""
	}

	// First, render plain text and ensure it's non-empty.
	var textBuf strings.Builder
	if err := article.RenderText(&textBuf); err != nil {
		return ""
	}

	text := strings.TrimSpace(textBuf.String())
	if len(text) == 0 {
		return ""
	}

	// Prefer the cleaned-up HTML from go-readability to preserve structure,
	// then fall back to plain text if needed.
	var htmlBuf strings.Builder
	if err := article.RenderHTML(&htmlBuf); err == nil {
		renderedHTML := strings.TrimSpace(htmlBuf.String())
		if renderedHTML != "" {
			return extractParagraphs(renderedHTML)
		}
	}
	return normalizeWhitespace(text)
}

func checkLength(text string) string {
	if len(text) < MinArticleLength {
		return ""
	}
	return text
}

// extractParagraphs extracts text from HTML while preserving paragraph structure.
// Paragraphs are separated by double newlines.
// It extracts paragraphs, headers, code blocks, and list items.
func extractParagraphs(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		// Fallback to simple tag stripping
		return normalizeWhitespace(StripTags(html))
	}

	var paragraphs []string

	// Extract headers (h1-h6)
	doc.Find("h1, h2, h3, h4, h5, h6").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	})

	// Extract paragraphs
	doc.Find("p").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	})

	// Extract code blocks
	doc.Find("pre code, pre").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	})

	// Extract list items
	doc.Find("li").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			paragraphs = append(paragraphs, text)
		}
	})

	// If no structured content found, try to extract from other block elements
	if len(paragraphs) == 0 {
		doc.Find("div, article, section").Each(func(i int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			// Only include meaningful content (at least 10 chars)
			if text != "" && len(text) > 10 {
				paragraphs = append(paragraphs, text)
			}
		})
	}

	// If still no content, fallback to simple tag stripping
	if len(paragraphs) == 0 {
		return normalizeWhitespace(StripTags(html))
	}

	// Join paragraphs with double newlines
	return strings.Join(paragraphs, "\n\n")
}

// Clean search results using goquery for better HTML handling
func CleanSearchResultsWithGoquery(feeds []*domain.FeedItem) []*domain.FeedItem {
	for _, feed := range feeds {
		// Clean description using goquery
		feed.Description = cleanHTMLWithGoquery(feed.Description)
	}
	return feeds
}

// Use goquery to clean HTML content intelligently
func cleanHTMLWithGoquery(raw string) string {
	// Handle empty content
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	// If no HTML tags, just clean and truncate
	if !strings.Contains(raw, "<") {
		return truncateText(strings.TrimSpace(raw))
	}

	// Parse HTML with goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		// Fallback to basic tag stripping
		return truncateText(strings.TrimSpace(StripTags(raw)))
	}

	// Remove script, style, and other non-content elements
	doc.Find("script, style, nav, header, footer, aside").Remove()

	// Extract text content with intelligent spacing
	var textParts []string

	// Get main content from paragraphs first
	doc.Find("p").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text != "" {
			textParts = append(textParts, text)
		}
	})

	// If no paragraphs found, get content from other elements
	if len(textParts) == 0 {
		doc.Find("div, article, section, span").Each(func(i int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if text != "" && len(text) > 10 { // Only meaningful content
				textParts = append(textParts, text)
			}
		})
	}

	// If still no content, get all text
	if len(textParts) == 0 {
		text := strings.TrimSpace(doc.Text())
		if text != "" {
			textParts = append(textParts, text)
		}
	}

	// Join content with proper spacing
	result := strings.Join(textParts, " ")

	// Clean up whitespace
	result = normalizeWhitespace(result)

	return truncateText(result)
}

// Normalize whitespace and remove extra spaces
func normalizeWhitespace(s string) string {
	// Replace multiple whitespace with single space
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// Truncate text to reasonable length for search results
func truncateText(s string) string {
	if len(s) <= constants.MaxSearchResultLength {
		return s
	}

	// Try to break at word boundary
	if idx := strings.LastIndex(s[:constants.MaxSearchResultLength], " "); idx > constants.MaxSearchResultLength-50 {
		return s[:idx] + "..."
	}

	return s[:constants.MaxSearchResultLength] + "..."
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

// ExtractTitle extracts the article title from HTML content.
// Priority order: <title> tag, og:title meta tag, first <h1> tag.
// Returns empty string if no title found.
func ExtractTitle(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err != nil {
		return ""
	}

	// 1. Try <title> tag first
	title := strings.TrimSpace(doc.Find("title").First().Text())
	if title != "" {
		return title
	}

	// 2. Try Open Graph title meta tag
	ogTitle, exists := doc.Find("meta[property='og:title']").First().Attr("content")
	if exists && strings.TrimSpace(ogTitle) != "" {
		return strings.TrimSpace(ogTitle)
	}

	// 3. Fall back to first <h1> tag
	h1Title := strings.TrimSpace(doc.Find("h1").First().Text())
	if h1Title != "" {
		return h1Title
	}

	// No title found
	return ""
}
