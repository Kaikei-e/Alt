package html_parser

import (
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

// ExtractOgImageURL extracts og:image URL from raw HTML.
// Only HTTPS URLs are returned; HTTP and other schemes are rejected.
func ExtractOgImageURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err != nil {
		return ""
	}

	ogImage, exists := doc.Find("meta[property='og:image']").First().Attr("content")
	if !exists {
		return ""
	}

	ogImage = strings.TrimSpace(ogImage)
	if ogImage == "" {
		return ""
	}

	// Only allow HTTPS URLs
	if !strings.HasPrefix(ogImage, "https://") {
		return ""
	}

	return ogImage
}
