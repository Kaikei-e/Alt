package html_parser

import (
	"encoding/json"
	"strings"

	"codeberg.org/readeck/go-readability/v2"
	"github.com/PuerkitoBio/goquery"
	"github.com/microcosm-cc/bluemonday"
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
		// Remove navigation, header, footer, aside
		doc.Find("head, script, style, noscript, title, aside, nav, header, footer").Remove()

		// Remove media and embedded content (ads, tracking, etc.)
		doc.Find("iframe, embed, object, video, audio, canvas").Remove()

		// Remove social media elements
		doc.Find("[class*='social'], [class*='share'], [class*='twitter'], [class*='facebook'], [class*='instagram'], [class*='linkedin']").Remove()
		doc.Find("[id*='social'], [id*='share'], [id*='twitter'], [id*='facebook']").Remove()

		// Remove comment sections
		doc.Find("[class*='comment'], [id*='comment'], [class*='discussion'], [id*='discussion']").Remove()

		// Remove metadata and resource links
		doc.Find("meta, link[rel='stylesheet'], link[rel='preload'], link[rel='prefetch'], link[rel='dns-prefetch']").Remove()

		// Remove inline styles and event handlers from all elements
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

		cleanedHTML, _ := doc.Html()
		if cleanedHTML != "" {
			trimmed = cleanedHTML
		}
	}

	// 3. Try go-readability on the cleaned document
	article, err := readability.FromReader(strings.NewReader(trimmed), nil)
	if err == nil {
		// First, render plain text and ensure it's non-empty.
		var textBuf strings.Builder
		if err := article.RenderText(&textBuf); err == nil {
			text := strings.TrimSpace(textBuf.String())

			// Validate extracted text length.
			// Sometimes readability extracts only the title or metadata (e.g. < 200 chars)
			// while the actual content is much larger.
			// If text is too short, we fallback to simple extraction.
			if len(text) >= 200 {
				// Prefer the cleaned-up HTML from go-readability to preserve structure,
				// then fall back to plain text if needed.
				var htmlBuf strings.Builder
				if err := article.RenderHTML(&htmlBuf); err == nil {
					html := strings.TrimSpace(htmlBuf.String())
					if html != "" {
						return extractParagraphs(html)
					}
				}
				return normalizeWhitespace(text)
			}
			// Fall through to fallback if text is too short
		}
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
	// If still no content, fallback to simple tag stripping
	if len(paragraphs) == 0 {
		p := bluemonday.StrictPolicy()
		return normalizeWhitespace(p.Sanitize(html))
	}

	// Join paragraphs with double newlines
	return strings.Join(paragraphs, "\n\n")
}

// StripTags removes HTML tags from a string and returns plain text.
// It uses bluemonday's strict policy which strips all tags.
func StripTags(raw string) string {
	p := bluemonday.StrictPolicy()
	return normalizeWhitespace(p.Sanitize(raw))
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
