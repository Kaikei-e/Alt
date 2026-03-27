package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyTier1(t *testing.T) {
	// Helper: generate plain text of exact length
	textOfLength := func(n int) string {
		if n <= 0 {
			return ""
		}
		return strings.Repeat("a", n)
	}

	tests := []struct {
		name    string
		content string
		url     string
		wantOK  bool
		wantSub string // substring expected in Reason when rejected
	}{
		// --- Length boundary ---
		{
			name:    "empty content is non-tier1",
			content: "",
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "length",
		},
		{
			name:    "499 chars is non-tier1",
			content: textOfLength(499),
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "length",
		},
		{
			name:    "500 chars is tier1",
			content: textOfLength(500),
			url:     "https://example.com/article",
			wantOK:  true,
		},
		{
			name:    "501 chars is tier1",
			content: textOfLength(501),
			url:     "https://example.com/article",
			wantOK:  true,
		},
		{
			name:    "HTML tags inflate raw length but plaintext is under 500",
			content: "<p>" + textOfLength(400) + "</p><div><span>" + textOfLength(50) + "</span></div>",
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "length",
		},
		{
			name:    "HTML tags inflate raw length but plaintext is 500+",
			content: "<p>" + textOfLength(500) + "</p>",
			url:     "https://example.com/article",
			wantOK:  true,
		},

		// --- Truncation markers ---
		{
			name:    "ends with 続きをみる",
			content: textOfLength(600) + "続きをみる",
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "truncat",
		},
		{
			name:    "ends with 続きを読む",
			content: textOfLength(600) + "続きを読む",
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "truncat",
		},
		{
			name:    "ends with Read more",
			content: textOfLength(600) + "Read more",
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "truncat",
		},
		{
			name:    "ends with Read More",
			content: textOfLength(600) + "Read More",
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "truncat",
		},
		{
			name:    "ends with ...",
			content: textOfLength(600) + "...",
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "truncat",
		},
		{
			name:    "ends with … (ellipsis char)",
			content: textOfLength(600) + "…",
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "truncat",
		},
		{
			name:    "truncation marker in middle does not reject",
			content: textOfLength(300) + "続きをみる" + textOfLength(300),
			url:     "https://example.com/article",
			wantOK:  true,
		},

		// --- Non-article URL patterns ---
		{
			name:    "crossword URL is non-tier1",
			content: textOfLength(1000),
			url:     "https://www.theguardian.com/crosswords/cryptic/29847",
			wantOK:  false,
			wantSub: "non-article URL",
		},
		{
			name:    "gallery URL is non-tier1",
			content: textOfLength(1000),
			url:     "https://www.theguardian.com/gallery/2025/some-photos",
			wantOK:  false,
			wantSub: "non-article URL",
		},
		{
			name:    "puzzles URL is non-tier1",
			content: textOfLength(1000),
			url:     "https://example.com/puzzles/daily",
			wantOK:  false,
			wantSub: "non-article URL",
		},

		// --- Placeholder / boilerplate ---
		{
			name:    "crosswords saved placeholder",
			content: "Crosswords are saved automatically.",
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "placeholder",
		},
		{
			name:    "what to read next boilerplate",
			content: "What to Read Next on Medscape",
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "placeholder",
		},
		{
			name:    "test placeholder",
			content: "test",
			url:     "https://zenn.dev/user/articles/test",
			wantOK:  false,
			wantSub: "placeholder",
		},
		{
			name:    "Discussion placeholder",
			content: "Discussion",
			url:     "https://zenn.dev/user/articles/abc",
			wantOK:  false,
			wantSub: "placeholder",
		},
		{
			name:    "はじめに続きをみる placeholder",
			content: "はじめに続きをみる",
			url:     "https://note.shiftinc.jp/article",
			wantOK:  false,
		},

		// --- Img-dominant content ---
		{
			name:    "img-dominant content with little text",
			content: `<p><img src="https://example.com/img.jpg" alt="photo"></p>Photography<br>Europe`,
			url:     "https://example.com/article",
			wantOK:  false,
			wantSub: "img-dominant",
		},
		{
			name:    "img with sufficient text is tier1",
			content: `<p><img src="https://example.com/img.jpg" alt="photo"></p>` + textOfLength(500),
			url:     "https://example.com/article",
			wantOK:  true,
		},

		// --- Normal full articles ---
		{
			name:    "normal 1000 char article is tier1",
			content: textOfLength(1000),
			url:     "https://example.com/good-article",
			wantOK:  true,
		},
		{
			name:    "long article with HTML is tier1",
			content: "<p>" + textOfLength(800) + "</p><p>" + textOfLength(800) + "</p>",
			url:     "https://dev.to/user/article-slug",
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyTier1(tt.content, tt.url)
			assert.Equal(t, tt.wantOK, result.IsTier1, "IsTier1 mismatch")
			if !tt.wantOK && tt.wantSub != "" {
				assert.Contains(t, result.Reason, tt.wantSub,
					"Reason should contain %q", tt.wantSub)
			}
			if tt.wantOK {
				assert.Empty(t, result.Reason, "Tier1 result should have empty Reason")
			}
		})
	}
}
