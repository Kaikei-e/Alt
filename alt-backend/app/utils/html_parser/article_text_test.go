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
