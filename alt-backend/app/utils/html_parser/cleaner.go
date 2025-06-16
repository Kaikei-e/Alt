package html_parser

import (
	"alt/domain"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

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
