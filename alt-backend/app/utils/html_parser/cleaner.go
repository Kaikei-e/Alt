package html_parser

import (
	"alt/domain"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ExtractArticleText converts raw article HTML into plain text paragraphs.
// It removes non-content elements (script/style/navigation) and normalizes
// whitespace so the returned string contains only readable sentences.
func ExtractArticleText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	// Short-circuit if the payload is already plain text.
	if !strings.Contains(trimmed, "<") {
		return normalizeWhitespace(trimmed)
	}

	// Parse HTML and fall back to simple stripping on failure.
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err != nil {
		return normalizeWhitespace(StripTags(trimmed))
	}

	// Remove elements that are unlikely to be part of the readable body.
	doc.Find("script, style, nav, header, footer, aside, form, iframe, noscript").Remove()

	collect := func(selection *goquery.Selection) []string {
		var parts []string
		selection.Each(func(_ int, s *goquery.Selection) {
			text := normalizeWhitespace(strings.TrimSpace(s.Text()))
			if text == "" {
				return
			}
			if len(parts) == 0 || parts[len(parts)-1] != text {
				parts = append(parts, text)
			}
		})
		return parts
	}

	paragraphs := collect(doc.Find("article p, main p, section p, div p, p"))
	if len(paragraphs) == 0 {
		paragraphs = collect(doc.Find("li, blockquote"))
	}
	if len(paragraphs) == 0 {
		paragraphs = collect(doc.Find("h1, h2, h3, h4, h5, h6"))
	}
	if len(paragraphs) == 0 {
		fallback := normalizeWhitespace(strings.TrimSpace(doc.Text()))
		if fallback != "" {
			paragraphs = append(paragraphs, fallback)
		}
	}

	if len(paragraphs) == 0 {
		return ""
	}

	return strings.TrimSpace(strings.Join(paragraphs, "\n\n"))
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
	const maxLength = 300 // Shorter for search results
	if len(s) <= maxLength {
		return s
	}

	// Try to break at word boundary
	if idx := strings.LastIndex(s[:maxLength], " "); idx > maxLength-50 {
		return s[:idx] + "..."
	}

	return s[:maxLength] + "..."
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
