package html_parser

import "testing"

func TestExtractArticleText_ReturnsPlainTextWhenHTMLProvided(t *testing.T) {
	raw := `<html><body><header>Ignore</header><p>First sentence.</p><p>Second sentence.</p><footer>Ignore</footer></body></html>`
	expected := "First sentence.\n\nSecond sentence."

	got := ExtractArticleText(raw)
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestExtractArticleText_FallsBackToStripTags(t *testing.T) {
	// Malformed HTML should still produce plain text
	raw := "<p>Broken"

	got := ExtractArticleText(raw)
	if got != "Broken" {
		t.Fatalf("expected fallback plain text, got %q", got)
	}
}

func TestExtractArticleText_RemovesScripts(t *testing.T) {
	raw := `<html><body><script>alert('x')</script><p>Visible</p></body></html>`

	got := ExtractArticleText(raw)
	if got != "Visible" {
		t.Fatalf("expected script to be removed, got %q", got)
	}
}

func TestExtractTitle_ExtractsFromTitleTag(t *testing.T) {
	raw := `<html><head><title>Article Title</title></head><body><p>Content</p></body></html>`
	expected := "Article Title"

	got := ExtractTitle(raw)
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestExtractTitle_FallsBackToFirstH1(t *testing.T) {
	raw := `<html><body><h1>Main Heading</h1><p>Content</p></body></html>`
	expected := "Main Heading"

	got := ExtractTitle(raw)
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestExtractTitle_ReturnsEmptyWhenNoTitle(t *testing.T) {
	raw := `<html><body><p>Just content</p></body></html>`

	got := ExtractTitle(raw)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestExtractTitle_TrimsWhitespace(t *testing.T) {
	raw := `<html><head><title>  Article Title  </title></head><body><p>Content</p></body></html>`
	expected := "Article Title"

	got := ExtractTitle(raw)
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestExtractTitle_HandlesOGTitle(t *testing.T) {
	raw := `<html><head><meta property="og:title" content="OG Title"/></head><body><p>Content</p></body></html>`
	expected := "OG Title"

	got := ExtractTitle(raw)
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}
