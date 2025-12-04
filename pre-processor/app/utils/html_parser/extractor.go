package html_parser

import (
	"encoding/json"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
	"golang.org/x/net/html"
)

// ExtractArticleText converts raw article HTML into plain text paragraphs.
// It removes non-content elements (script/style/navigation) and normalizes
// whitespace so the returned string contains only readable sentences.
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

	// Prepare goquery document for further inspection
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err == nil {
		// 1. Check for Next.js __NEXT_DATA__ script first (highest priority)
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
								text := extractParagraphs(bodyHtml)
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

		// 2. Pre-process HTML: Remove non-content elements before go-readability
		doc.Find("head, script, style, noscript, title, aside, nav, header, footer").Remove()
		cleanedHTML, _ := doc.Html()
		if cleanedHTML != "" {
			trimmed = cleanedHTML
		}
	}

	// 3. Try go-readability on the cleaned document
	article, err := readability.FromReader(strings.NewReader(trimmed), nil)
	if err == nil && len(strings.TrimSpace(article.TextContent)) > 0 {
		// Use go-readability result, but preserve paragraph structure
		// article.Content is the HTML content, article.TextContent is the plain text
		// We'll extract paragraphs from the HTML content to preserve structure
		if article.Content != "" {
			return extractParagraphs(article.Content)
		}
		// Fallback to TextContent if Content is empty
		return normalizeWhitespace(article.TextContent)
	}

	// 4. Final fallback: Strip tags from the original HTML
	return extractParagraphs(trimmed)
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

// StripTags removes HTML tags from a string and returns plain text.
// It skips script and style tags automatically.
func StripTags(raw string) string {
	return stripCore(strings.NewReader(raw))
}

// stripCore is the internal implementation of tag stripping.
func stripCore(r *strings.Reader) string {
	var b strings.Builder
	z := html.NewTokenizer(r)

	depthSkip := 0 // <script> や <style> ブロックを無視するための深さカウンタ

	for {
		switch tt := z.Next(); tt {
		case html.ErrorToken:
			return normalizeWhitespace(b.String())

		case html.StartTagToken:
			name, _ := z.TagName()
			if skipTag(name) {
				depthSkip++
			}

		case html.EndTagToken:
			name, _ := z.TagName()
			if skipTag(name) && depthSkip > 0 {
				depthSkip--
			}

		case html.TextToken:
			if depthSkip == 0 { // script/style 内はスキップ
				b.Write(z.Text())
			}
		}
	}
}

// skipTag checks if a tag should be skipped (script, style, noscript).
func skipTag(name []byte) bool {
	switch string(name) {
	case "script", "style", "noscript":
		return true
	default:
		return false
	}
}

// normalizeWhitespace normalizes whitespace by replacing multiple spaces with single space.
func normalizeWhitespace(s string) string {
	// Replace multiple whitespace with single space
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
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
