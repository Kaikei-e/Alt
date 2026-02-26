package image_proxy

import (
	"testing"
)

func TestSigner_GenerateAndVerify(t *testing.T) {
	signer := NewSigner("test-secret-key")

	tests := []struct {
		name     string
		imageURL string
	}{
		{
			name:     "basic HTTPS URL",
			imageURL: "https://example.com/image.jpg",
		},
		{
			name:     "URL with query params",
			imageURL: "https://cdn.example.com/photo.webp?w=1200&h=630",
		},
		{
			name:     "URL with unicode path",
			imageURL: "https://example.com/画像/photo.png",
		},
		{
			name:     "URL with special chars",
			imageURL: "https://example.com/images/test%20image.jpg?q=80&format=webp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxyURL := signer.GenerateProxyURL(tt.imageURL)
			if proxyURL == "" {
				t.Fatal("GenerateProxyURL returned empty string")
			}

			// Extract sig and encodedURL from the proxy URL
			sig, encodedURL := parseProxyURL(t, proxyURL)

			decodedURL, err := signer.VerifyAndDecode(sig, encodedURL)
			if err != nil {
				t.Fatalf("VerifyAndDecode failed: %v", err)
			}
			if decodedURL != tt.imageURL {
				t.Errorf("URL mismatch: got %q, want %q", decodedURL, tt.imageURL)
			}
		})
	}
}

func TestSigner_VerifyRejectsInvalidSignature(t *testing.T) {
	signer := NewSigner("test-secret-key")

	proxyURL := signer.GenerateProxyURL("https://example.com/image.jpg")
	_, encodedURL := parseProxyURL(t, proxyURL)

	// Tamper with signature
	_, err := signer.VerifyAndDecode("0000000000000000000000000000000000000000000000000000000000000000", encodedURL)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestSigner_VerifyRejectsDifferentSecret(t *testing.T) {
	signer1 := NewSigner("secret-1")
	signer2 := NewSigner("secret-2")

	proxyURL := signer1.GenerateProxyURL("https://example.com/image.jpg")
	sig, encodedURL := parseProxyURL(t, proxyURL)

	_, err := signer2.VerifyAndDecode(sig, encodedURL)
	if err == nil {
		t.Fatal("expected error for different secret")
	}
}

func TestSigner_GenerateProxyURLEmptyInput(t *testing.T) {
	signer := NewSigner("test-secret")
	result := signer.GenerateProxyURL("")
	if result != "" {
		t.Errorf("expected empty string for empty input, got %q", result)
	}
}

func TestSigner_VerifyAndDecodeInvalidBase64(t *testing.T) {
	signer := NewSigner("test-secret")
	_, err := signer.VerifyAndDecode("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", "!!!not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestURLHash(t *testing.T) {
	hash := URLHash("https://example.com/image.jpg")
	if hash == "" {
		t.Fatal("URLHash returned empty string")
	}
	if len(hash) != 64 {
		t.Errorf("expected SHA-256 hex length 64, got %d", len(hash))
	}

	// Same input should produce same hash
	hash2 := URLHash("https://example.com/image.jpg")
	if hash != hash2 {
		t.Error("URLHash is not deterministic")
	}

	// Different input should produce different hash
	hash3 := URLHash("https://example.com/other.jpg")
	if hash == hash3 {
		t.Error("different URLs produced same hash")
	}
}

// parseProxyURL extracts sig and encodedURL from a proxy URL path.
func parseProxyURL(t *testing.T, proxyURL string) (string, string) {
	t.Helper()
	// proxyURL format: /v1/images/proxy/{sig}/{encodedURL}
	const prefix = "/v1/images/proxy/"
	if len(proxyURL) <= len(prefix) {
		t.Fatalf("proxy URL too short: %q", proxyURL)
	}
	rest := proxyURL[len(prefix):]

	// Find the separator between sig and encodedURL
	slashIdx := -1
	for i, c := range rest {
		if c == '/' {
			slashIdx = i
			break
		}
	}
	if slashIdx < 0 {
		t.Fatalf("no separator found in proxy URL: %q", proxyURL)
	}

	return rest[:slashIdx], rest[slashIdx+1:]
}
