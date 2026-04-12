package domain

import "testing"

func TestIsValidImageContentType_Whitelist(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{"empty", "", false},
		{"jpeg", "image/jpeg", true},
		{"jpeg with charset", "image/jpeg; charset=binary", true},
		{"png", "image/png", true},
		{"gif", "image/gif", true},
		{"webp", "image/webp", true},
		{"avif not yet supported", "image/avif", false},
		{"heic not yet supported", "image/heic", false},
		{"jxl not yet supported", "image/jxl", false},
		{"svg blocked (XSS)", "image/svg+xml", false},
		{"html error page", "text/html", false},
		{"uppercase jpeg", "IMAGE/JPEG", true},
		{"with whitespace", "  image/png  ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidImageContentType(tt.contentType)
			if got != tt.want {
				t.Errorf("IsValidImageContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}
