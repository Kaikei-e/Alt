package html_parser

import (
	"testing"
)

// =============================================================================
// ExtractOgImageURL Tests
// =============================================================================

const testBaseURL = "https://example.com/articles/the-post"

func TestExtractOgImageURL_HTTPS(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="https://example.com/image.jpg" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, testBaseURL)
	if result != "https://example.com/image.jpg" {
		t.Errorf("Expected https://example.com/image.jpg, got %q", result)
	}
}

// HTTP og:image is now accepted: the image proxy fetches it server-side and
// re-serves it over HTTPS, so there is no mixed-content concern for the client.
func TestExtractOgImageURL_HTTP_Allowed(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="http://example.com/image.jpg" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, testBaseURL)
	if result != "http://example.com/image.jpg" {
		t.Errorf("Expected http URL to be accepted, got %q", result)
	}
}

func TestExtractOgImageURL_ProtocolRelative(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="//cdn.example.com/image.jpg" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, testBaseURL)
	if result != "https://cdn.example.com/image.jpg" {
		t.Errorf("Expected protocol-relative URL resolved to https, got %q", result)
	}
}

func TestExtractOgImageURL_RelativePath(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="/media/cover.png" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, testBaseURL)
	if result != "https://example.com/media/cover.png" {
		t.Errorf("Expected relative path resolved against base, got %q", result)
	}
}

func TestExtractOgImageURL_SecureURLPreferred(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="http://example.com/plain.jpg" />
		<meta property="og:image:secure_url" content="https://example.com/secure.jpg" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, testBaseURL)
	if result != "https://example.com/secure.jpg" {
		t.Errorf("Expected og:image:secure_url to be preferred, got %q", result)
	}
}

func TestExtractOgImageURL_TwitterImageFallback(t *testing.T) {
	raw := `<html><head>
		<meta name="twitter:image" content="https://example.com/twitter.jpg" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, testBaseURL)
	if result != "https://example.com/twitter.jpg" {
		t.Errorf("Expected twitter:image fallback, got %q", result)
	}
}

func TestExtractOgImageURL_LinkImageSrcFallback(t *testing.T) {
	raw := `<html><head>
		<link rel="image_src" href="https://example.com/legacy.jpg" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, testBaseURL)
	if result != "https://example.com/legacy.jpg" {
		t.Errorf("Expected link[rel=image_src] fallback, got %q", result)
	}
}

// A data: URI candidate is unusable; extraction must fall through to the next
// candidate rather than returning the data URI.
func TestExtractOgImageURL_DataURIFallsThrough(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="data:image/png;base64,iVBORw0KGgo=" />
		<meta name="twitter:image" content="https://example.com/real.jpg" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, testBaseURL)
	if result != "https://example.com/real.jpg" {
		t.Errorf("Expected fall-through to twitter:image past data URI, got %q", result)
	}
}

func TestExtractOgImageURL_Missing(t *testing.T) {
	raw := `<html><head>
		<meta property="og:title" content="Title" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, testBaseURL)
	if result != "" {
		t.Errorf("Expected empty string when og:image missing, got %q", result)
	}
}

func TestExtractOgImageURL_EmptyContent(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, testBaseURL)
	if result != "" {
		t.Errorf("Expected empty string for empty content, got %q", result)
	}
}

func TestExtractOgImageURL_EmptyInput(t *testing.T) {
	result := ExtractOgImageURL("", testBaseURL)
	if result != "" {
		t.Errorf("Expected empty string for empty input, got %q", result)
	}
}

// Without a base URL a relative candidate cannot be resolved and must be skipped.
func TestExtractOgImageURL_RelativeWithoutBaseSkipped(t *testing.T) {
	raw := `<html><head>
		<meta property="og:image" content="/media/cover.png" />
	</head><body><p>Content</p></body></html>`

	result := ExtractOgImageURL(raw, "")
	if result != "" {
		t.Errorf("Expected empty string when relative URL cannot be resolved, got %q", result)
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
