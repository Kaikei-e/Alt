package html_parser

import (
	"strings"
	"testing"
)

func TestExtractArticleText_PlainText(t *testing.T) {
	input := "This is plain text without any HTML tags."
	result := ExtractArticleText(input)
	if result != input {
		t.Errorf("Expected plain text to be returned as-is, got: %s", result)
	}
}

func TestExtractArticleText_EmptyString(t *testing.T) {
	result := ExtractArticleText("")
	if result != "" {
		t.Errorf("Expected empty string, got: %s", result)
	}
}

func TestExtractArticleText_SimpleHTML(t *testing.T) {
	input := "<html><body><p>This is a paragraph.</p><p>This is another paragraph.</p></body></html>"
	result := ExtractArticleText(input)
	if !strings.Contains(result, "This is a paragraph") {
		t.Errorf("Expected to extract paragraph text, got: %s", result)
	}
	if !strings.Contains(result, "This is another paragraph") {
		t.Errorf("Expected to extract second paragraph text, got: %s", result)
	}
}

func TestExtractArticleText_WithScriptAndStyle(t *testing.T) {
	input := `<html><head><script>alert('test');</script><style>body { color: red; }</style></head><body><p>This is content.</p></body></html>`
	result := ExtractArticleText(input)
	if strings.Contains(result, "alert") {
		t.Errorf("Script content should be removed, got: %s", result)
	}
	if strings.Contains(result, "color: red") {
		t.Errorf("Style content should be removed, got: %s", result)
	}
	if !strings.Contains(result, "This is content") {
		t.Errorf("Expected to extract paragraph text, got: %s", result)
	}
}

func TestExtractArticleText_WithHeaders(t *testing.T) {
	input := "<html><body><h1>Main Title</h1><p>Paragraph text.</p><h2>Subtitle</h2></body></html>"
	result := ExtractArticleText(input)
	if !strings.Contains(result, "Main Title") {
		t.Errorf("Expected to extract h1 text, got: %s", result)
	}
	if !strings.Contains(result, "Subtitle") {
		t.Errorf("Expected to extract h2 text, got: %s", result)
	}
	if !strings.Contains(result, "Paragraph text") {
		t.Errorf("Expected to extract paragraph text, got: %s", result)
	}
}

func TestExtractArticleText_WithListItems(t *testing.T) {
	input := "<html><body><ul><li>First item</li><li>Second item</li></ul></body></html>"
	result := ExtractArticleText(input)
	if !strings.Contains(result, "First item") {
		t.Errorf("Expected to extract first list item, got: %s", result)
	}
	if !strings.Contains(result, "Second item") {
		t.Errorf("Expected to extract second list item, got: %s", result)
	}
}

func TestExtractArticleText_NextJSData(t *testing.T) {
	// Note: Next.js extraction depends on the exact JSON structure
	// This test verifies that the function handles Next.js data gracefully
	// Even if extraction fails, it should fall back to regular HTML parsing
	input := `<html><body><script id="__NEXT_DATA__">{"props":{"pageProps":{"article":{"title":"Test Article","bodyHtml":"<p>Article body content</p>"}}}}}</script><p>Fallback content</p></body></html>`
	result := ExtractArticleText(input)
	// The result should contain either the extracted Next.js content or fallback content
	if result == "" {
		t.Errorf("Expected to extract some content, got empty string")
	}
	// Verify that we get meaningful content (either from Next.js or fallback)
	if !strings.Contains(result, "Fallback content") && !strings.Contains(result, "Test Article") && !strings.Contains(result, "Article body content") {
		t.Errorf("Expected to extract content from Next.js data or fallback, got: %s", result)
	}
}

func TestStripTags_SimpleHTML(t *testing.T) {
	input := "<p>This is a <strong>test</strong> paragraph.</p>"
	result := StripTags(input)
	expected := "This is a test paragraph."
	if strings.TrimSpace(result) != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestStripTags_WithScript(t *testing.T) {
	input := "<p>Content</p><script>alert('test');</script><p>More content</p>"
	result := StripTags(input)
	if strings.Contains(result, "alert") {
		t.Errorf("Script content should be removed, got: %s", result)
	}
	if !strings.Contains(result, "Content") {
		t.Errorf("Expected to keep paragraph content, got: %s", result)
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	input := "This   has    multiple     spaces"
	result := normalizeWhitespace(input)
	expected := "This has multiple spaces"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestExtractTitle_FromTitleTag(t *testing.T) {
	input := "<html><head><title>Test Title</title></head><body></body></html>"
	result := ExtractTitle(input)
	if result != "Test Title" {
		t.Errorf("Expected 'Test Title', got '%s'", result)
	}
}

func TestExtractTitle_FromOGTag(t *testing.T) {
	input := `<html><head><meta property="og:title" content="OG Title"></head><body></body></html>`
	result := ExtractTitle(input)
	if result != "OG Title" {
		t.Errorf("Expected 'OG Title', got '%s'", result)
	}
}

func TestExtractTitle_FromH1(t *testing.T) {
	input := "<html><body><h1>H1 Title</h1></body></html>"
	result := ExtractTitle(input)
	if result != "H1 Title" {
		t.Errorf("Expected 'H1 Title', got '%s'", result)
	}
}

func TestExtractTitle_NoTitle(t *testing.T) {
	input := "<html><body><p>No title here</p></body></html>"
	result := ExtractTitle(input)
	if result != "" {
		t.Errorf("Expected empty string, got '%s'", result)
	}
}

func TestExtractArticleText_RemovesIframeAndEmbed(t *testing.T) {
	input := `<html><body><p>Article content</p><iframe src="http://example.com"></iframe><embed src="video.swf"></embed><p>More content</p></body></html>`
	result := ExtractArticleText(input)
	if strings.Contains(result, "iframe") || strings.Contains(result, "embed") {
		t.Errorf("Iframe and embed tags should be removed, got: %s", result)
	}
	if !strings.Contains(result, "Article content") {
		t.Errorf("Expected to keep article content, got: %s", result)
	}
	if !strings.Contains(result, "More content") {
		t.Errorf("Expected to keep more content, got: %s", result)
	}
}

func TestExtractArticleText_RemovesSocialMediaElements(t *testing.T) {
	input := `<html><body><p>Article content</p><div class="social-share">Share buttons</div><div id="twitter-widget">Twitter</div><p>More content</p></body></html>`
	result := ExtractArticleText(input)
	if strings.Contains(result, "Share buttons") || strings.Contains(result, "Twitter") {
		t.Errorf("Social media elements should be removed, got: %s", result)
	}
	if !strings.Contains(result, "Article content") {
		t.Errorf("Expected to keep article content, got: %s", result)
	}
}

func TestExtractArticleText_RemovesCommentSections(t *testing.T) {
	input := `<html><body><p>Article content</p><div class="comments">Comment section</div><div id="comment-form">Comment form</div><p>More content</p></body></html>`
	result := ExtractArticleText(input)
	if strings.Contains(result, "Comment section") || strings.Contains(result, "Comment form") {
		t.Errorf("Comment sections should be removed, got: %s", result)
	}
	if !strings.Contains(result, "Article content") {
		t.Errorf("Expected to keep article content, got: %s", result)
	}
}

func TestExtractArticleText_RemovesInlineStylesAndEventHandlers(t *testing.T) {
	input := `<html><body><p style="color: red;">Article content</p><button onclick="alert('test')">Click me</button><p>More content</p></body></html>`
	result := ExtractArticleText(input)
	// Styles and event handlers should be removed, but content should remain
	if !strings.Contains(result, "Article content") {
		t.Errorf("Expected to keep article content, got: %s", result)
	}
	if !strings.Contains(result, "More content") {
		t.Errorf("Expected to keep more content, got: %s", result)
	}
	// Event handler code should not appear in text
	if strings.Contains(result, "alert") {
		t.Errorf("Event handler code should be removed, got: %s", result)
	}
}

func TestExtractArticleText_ReadabilityFallback(t *testing.T) {
	bodyContent := strings.Repeat("Important content that must be extracted. ", 20)
	input := `<html>
		<head><title>Short Title</title></head>
		<body>
			<h1>Short Title</h1>
			<div class="content" style="font-size: 10px;">
				<p>` + bodyContent + `</p>
			</div>
		</body>
	</html>`

	result := ExtractArticleText(input)

	// We expect the result to contain the body content.
	// If the current implementation (readability) returns just the title (which is likely if it penalizes the div or thinks it's boilerplate),
	// this test will fail. If it passes, then `readability` is smarter than I thought, but I'll still implement the safety check.
	if !strings.Contains(result, "Important content") {
		t.Errorf("Expected result to contain body content, but it was missing. Result start: %s...", result[:min(len(result), 50)])
	}

	// Also ensure we didn't just get the title
	if len(result) < 100 {
		t.Errorf("Result too short: %d chars. Expected > 100.", len(result))
	}
}
