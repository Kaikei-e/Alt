package domain

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

const minTier1Length = 500

var truncationMarkers = []string{
	"続きをみる",
	"続きを読む",
	"Read more",
	"Read More",
	"...",
	"…",
}

var nonArticleURLPatterns = []string{
	"/crosswords/",
	"/crossword/",
	"/gallery/",
	"/puzzles/",
}

var placeholderPrefixes = []string{
	"Crosswords are saved",
	"What to Read Next",
	"はじめに続きをみる",
}

var placeholderExact = []string{
	"test",
	"Discussion",
}

// ClassifyResult holds the Tier1 classification outcome.
type ClassifyResult struct {
	IsTier1 bool
	Reason  string // rejection reason (empty if Tier1)
}

// ClassifyTier1 determines if article content qualifies as Tier1.
// Tier1 = content that is worth persisting as a full article.
// Pure function: no I/O, no side effects.
func ClassifyTier1(content string, url string) ClassifyResult {
	if r := checkNonArticleURL(url); r != nil {
		return *r
	}

	plainText := stripTags(content)

	if r := checkPlaceholder(plainText); r != nil {
		return *r
	}

	if r := checkTruncationMarker(plainText); r != nil {
		return *r
	}

	if r := checkImgDominant(content, plainText); r != nil {
		return *r
	}

	if len([]rune(plainText)) < minTier1Length {
		return ClassifyResult{IsTier1: false, Reason: "content length below minimum"}
	}

	return ClassifyResult{IsTier1: true}
}

func stripTags(html string) string {
	p := bluemonday.StrictPolicy()
	stripped := p.Sanitize(html)
	fields := strings.Fields(stripped)
	return strings.Join(fields, " ")
}

func checkNonArticleURL(url string) *ClassifyResult {
	lower := strings.ToLower(url)
	for _, pattern := range nonArticleURLPatterns {
		if strings.Contains(lower, pattern) {
			return &ClassifyResult{IsTier1: false, Reason: "non-article URL pattern: " + pattern}
		}
	}
	return nil
}

func checkPlaceholder(plainText string) *ClassifyResult {
	trimmed := strings.TrimSpace(plainText)

	for _, exact := range placeholderExact {
		if trimmed == exact {
			return &ClassifyResult{IsTier1: false, Reason: "placeholder content"}
		}
	}

	for _, prefix := range placeholderPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return &ClassifyResult{IsTier1: false, Reason: "placeholder content"}
		}
	}

	return nil
}

func checkTruncationMarker(plainText string) *ClassifyResult {
	trimmed := strings.TrimSpace(plainText)
	for _, marker := range truncationMarkers {
		if strings.HasSuffix(trimmed, marker) {
			return &ClassifyResult{IsTier1: false, Reason: "truncated content (ends with " + marker + ")"}
		}
	}
	return nil
}

func checkImgDominant(rawHTML string, plainText string) *ClassifyResult {
	imgCount := strings.Count(strings.ToLower(rawHTML), "<img")
	if imgCount == 0 {
		return nil
	}

	textLen := len([]rune(plainText))
	// If there are images and text is very short, it's img-dominant
	if textLen < minTier1Length && imgCount > 0 {
		// This will be caught by the length check later anyway,
		// but we specifically flag img-dominant when images are present
		// and text is under threshold
		return &ClassifyResult{IsTier1: false, Reason: "img-dominant content with insufficient text"}
	}

	return nil
}
