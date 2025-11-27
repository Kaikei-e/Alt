package html_parser

import (
	"alt/domain"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
)

// ExtractArticleText converts raw article HTML into plain text paragraphs.
// It removes non-content elements (script/style/navigation) and normalizes
// whitespace so the returned string contains only readable sentences.
// ExtractArticleText converts raw article HTML into plain text paragraphs.
// It uses go-readability to extract the main content and then converts it to plain text.
func ExtractArticleText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	// Short-circuit if the payload is already plain text.
	if !strings.Contains(trimmed, "<") {
		return normalizeWhitespace(trimmed)
	}

	// 1. Try go-readability on the whole document
	article, err := readability.FromReader(strings.NewReader(trimmed), nil)
	if err != nil {
		// 2. Fallback: Try go-readability on the main content
		fmt.Println("go-readability failed on the whole document. Err: ", err)
	}
	// Prepare goquery document for further inspection
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err == nil {
		// 3. Fallback: Check for Next.js __NEXT_DATA__ script
		// Next.js sites often store the full article content in this JSON script
		nextData := doc.Find("script[id='__NEXT_DATA__']")
		if nextData.Length() > 0 {
			jsonData := nextData.Text()
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(jsonData), &data); err == nil {
				// Traverse: props -> pageProps -> article -> bodyHtml
				if props, ok := data["props"].(map[string]interface{}); ok {
					if pageProps, ok := props["pageProps"].(map[string]interface{}); ok {
						if articleData, ok := pageProps["article"].(map[string]interface{}); ok {
							// Extract title
							title, _ := articleData["title"].(string)

							// Extract body
							if bodyHtml, ok := articleData["bodyHtml"].(string); ok && len(bodyHtml) > 0 {
								// Since we found the specific body HTML, we don't need full readability parsing.
								// Just strip tags to get the text.
								text := normalizeWhitespace(StripTags(bodyHtml))
								if len(text) > 0 {
									if title != "" {
										return title + "\n\n" + text
									}
									return text
								}
							}
						}
					}
				}
			}
		}
	}

	// If step 1 succeeded but we didn't return (because we wanted to check others? No, currently step 1 returns immediately).
	// The issue is likely Step 1 returning garbage.
	// Let's modify the logic: If Step 1 returns something, but we have __NEXT_DATA__ with a specific structure,
	// maybe we should prioritize __NEXT_DATA__?
	// Or maybe Step 1 is returning the sidebar which is > 100 chars.

	if err == nil && len(strings.TrimSpace(article.TextContent)) > 100 {
		return normalizeWhitespace(article.TextContent)
	}

	// 4. Final fallback: Strip tags from the original HTML
	return normalizeWhitespace(StripTags(trimmed))

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
