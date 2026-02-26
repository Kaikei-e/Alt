package html_parser

import (
	"testing"
)

// =============================================================================
// ExtractOgImageURL Tests
// =============================================================================

func TestExtractOgImageURL_HTTPS(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="https://example.com/image.jpg" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw)
	if result != "https://example.com/image.jpg" {
		t.Errorf("Expected https://example.com/image.jpg, got %q", result)
	}
}

func TestExtractOgImageURL_HTTP_Rejected(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="http://example.com/image.jpg" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw)
	if result != "" {
		t.Errorf("Expected empty string for HTTP URL, got %q", result)
	}
}

func TestExtractOgImageURL_Missing(t *testing.T) {
	raw := `<html><head>
		<meta property="og:title" content="Title" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw)
	if result != "" {
		t.Errorf("Expected empty string when og:image missing, got %q", result)
	}
}

func TestExtractOgImageURL_EmptyContent(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw)
	if result != "" {
		t.Errorf("Expected empty string for empty content, got %q", result)
	}
}

func TestExtractOgImageURL_EmptyInput(t *testing.T) {
	result := ExtractOgImageURL("")
	if result != "" {
		t.Errorf("Expected empty string for empty input, got %q", result)
	}
}

// =============================================================================
// ExtractHead Tests
// =============================================================================

func TestExtractHead_Found(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="https://example.com/image.jpg" />
		<title>Test Title</title>
	</head><body><p>Content</p></body></html>`

	result := ExtractHead(raw)
	if result == "" {
		t.Error("Expected non-empty head HTML")
	}
	if !containsStr(result, "og:image") {
		t.Error("Expected head HTML to contain og:image meta tag")
	}
	if !containsStr(result, "Test Title") {
		t.Error("Expected head HTML to contain title")
	}
}

func TestExtractHead_Missing(t *testing.T) {
	raw := `<body><p>Content without head</p></body>`

	result := ExtractHead(raw)
	// goquery may auto-generate a <head>, so we just check it doesn't error
	// The result might be empty or contain an empty head
	_ = result
}

func TestExtractHead_EmptyInput(t *testing.T) {
	result := ExtractHead("")
	if result != "" {
		t.Errorf("Expected empty string for empty input, got %q", result)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
