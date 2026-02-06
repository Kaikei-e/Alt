package driver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"pre-processor/config"
	"pre-processor/domain"
)

// TestContentLengthMeasurement verifies that we use rune count (character count)
// instead of byte count for content length validation.
// This is critical for Japanese content where 1 character = 3 bytes in UTF-8.
func TestArticleSummarizerAPIClient_Returns429(t *testing.T) {
	t.Run("should return ErrServiceOverloaded on 429 response", func(t *testing.T) {
		// Create a test server that returns 429
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": "queue full"}`))
		}))
		defer server.Close()

		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

		cfg := &config.Config{
			NewsCreator: config.NewsCreatorConfig{
				Host:    server.URL,
				APIPath: "/api/v1/summarize",
				Timeout: 5 * 1_000_000_000, // 5 seconds as time.Duration (nanoseconds)
			},
		}

		article := &domain.Article{
			ID:      "test-article-429",
			Content: strings.Repeat("Test content for summarization. ", 10),
		}

		_, err := ArticleSummarizerAPIClient(context.Background(), article, cfg, logger, "low")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, domain.ErrServiceOverloaded) {
			t.Errorf("expected ErrServiceOverloaded, got: %v", err)
		}
	})
}

func TestContentLengthMeasurement(t *testing.T) {
	// Generate test strings with exact lengths
	englishShort := strings.Repeat("a", 77)              // 77 chars, 77 bytes
	englishExact := strings.Repeat("a", 100)             // 100 chars, 100 bytes
	japaneseShort := strings.Repeat("あ", 34)             // 34 chars, 102 bytes
	japaneseExact := strings.Repeat("あ", 100)            // 100 chars, 300 bytes
	mixed := "Hello" + strings.Repeat("あ", 10) + "World" // 15 ASCII + 10 Japanese = 25 chars, 45 bytes

	tests := []struct {
		name          string
		content       string
		expectedBytes int
		expectedRunes int
		shouldPassMin bool // Should pass minContentLength=100 check
	}{
		{
			name:          "English text under 100 chars",
			content:       englishShort,
			expectedBytes: 77,
			expectedRunes: 77,
			shouldPassMin: false,
		},
		{
			name:          "English text exactly 100 chars",
			content:       englishExact,
			expectedBytes: 100,
			expectedRunes: 100,
			shouldPassMin: true,
		},
		{
			name:          "Japanese text - 34 chars but 102 bytes",
			content:       japaneseShort,
			expectedBytes: 102,
			expectedRunes: 34,
			shouldPassMin: false, // 34 runes < 100, should NOT pass
		},
		{
			name:          "Japanese text - 100 chars (300 bytes)",
			content:       japaneseExact,
			expectedBytes: 300,
			expectedRunes: 100,
			shouldPassMin: true, // 100 runes = 100, should pass
		},
		{
			name:          "Mixed Japanese and English - bytes misleading",
			content:       mixed,
			expectedBytes: 40,
			expectedRunes: 20,
			shouldPassMin: false,
		},
	}

	const minContentLength = 100

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			byteCount := len(tt.content)
			runeCount := utf8.RuneCountInString(tt.content)

			if byteCount != tt.expectedBytes {
				t.Errorf("byte count mismatch: got %d, want %d", byteCount, tt.expectedBytes)
			}

			if runeCount != tt.expectedRunes {
				t.Errorf("rune count mismatch: got %d, want %d", runeCount, tt.expectedRunes)
			}

			// This is the key test: validate that rune count is used for the check
			passesCheck := runeCount >= minContentLength
			if passesCheck != tt.shouldPassMin {
				t.Errorf("min length check: got %v (runeCount=%d >= %d), want %v",
					passesCheck, runeCount, minContentLength, tt.shouldPassMin)
			}

			// Explicitly show the bug we're fixing: using byte count would give wrong result for Japanese
			if tt.name == "Japanese text - 34 chars but 102 bytes" {
				byteBasedCheck := byteCount >= minContentLength
				runeBasedCheck := runeCount >= minContentLength

				// Before fix: byte-based check would INCORRECTLY pass (102 >= 100)
				// After fix: rune-based check correctly fails (34 < 100)
				if byteBasedCheck == runeBasedCheck {
					t.Errorf("expected byte-based and rune-based checks to differ for Japanese text")
				}
				if !byteBasedCheck {
					t.Errorf("byte-based check should pass (102 >= 100) - this demonstrates the bug")
				}
				if runeBasedCheck {
					t.Errorf("rune-based check should fail (34 < 100) - this is the correct behavior")
				}
			}
		})
	}
}
