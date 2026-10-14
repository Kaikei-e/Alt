// Package tagclean normalizes and filters auto-generated article tags before
// they are displayed on the trail or used for episode chaining. ML-generated
// tags arrive unmoderated (ADR-000182 lineage): stopwords, digit-only
// fragments, URL debris and case/plural variants all reach the read path, so
// display-bound tags pass through this pipeline instead of being surfaced raw.
package tagclean

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// junkWords holds exact-match tags that carry no filtering power: English
// function words that leak out of keyword extraction, Japanese function
// words, and URL/HTML debris. Lowercase only — Normalize lowercases first.
var junkWords = map[string]struct{}{
	// English function/filler words seen verbatim in production tag sets.
	"also": {}, "could": {}, "might": {}, "would": {}, "said": {}, "says": {},
	"wrote": {}, "becomes": {}, "without": {}, "even": {}, "great": {},
	"three": {}, "week": {}, "types": {}, "example": {},
	// High-document-frequency generic tech/news meta-tags that cause Union-Find
	// megaclusters across unrelated articles.
	"ai": {}, "llm": {}, "api": {}, "code": {}, "data": {}, "user": {}, "users": {},
	"model": {}, "models": {}, "system": {}, "systems": {}, "tool": {}, "tools": {},
	"app": {}, "apps": {}, "web": {}, "blog": {}, "news": {}, "tech": {}, "software": {},
	// Japanese function words & generic meta-tags.
	"こと": {}, "もの": {}, "ため": {}, "よう": {}, "それ": {}, "これ": {},
	"情報": {}, "技術": {}, "開発": {}, "設計": {}, "実装": {}, "処理": {}, "構造": {},
	"利用": {}, "機能": {}, "記事": {}, "解説": {}, "紹介": {}, "方法": {}, "対応": {},
	"更新": {}, "問題": {}, "結果": {}, "サービス": {}, "システム": {}, "データ": {},
	"ユーザー": {}, "ツール": {}, "モデル": {}, "コード": {}, "アプリ": {},
	"エンジニア": {}, "プログラマー": {},
	// URL / HTML debris.
	"https": {}, "http": {}, "www": {}, "com": {}, "gt": {}, "lt": {}, "amp": {},
}

// Normalize returns the canonical form of one tag, or "" when the tag is
// junk: a stopword, contains no letters, shorter than two runes, or URL/HTML debris.
func Normalize(tag string) string {
	t := strings.ToLower(strings.TrimSpace(tag))
	if utf8.RuneCountInString(t) < 2 {
		return ""
	}
	if !hasLetter(t) {
		return ""
	}
	if _, junk := junkWords[t]; junk {
		return ""
	}
	return t
}

// CleanDisplay normalizes every tag, drops junk, and merges duplicates —
// including case variants and naive English singular/plural pairs — while
// preserving first-seen order.
func CleanDisplay(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	ordered := make([]string, 0, len(tags))
	for _, raw := range tags {
		t := Normalize(raw)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		ordered = append(ordered, t)
	}

	// A plural whose singular is also present is a variant, not a second tag.
	out := ordered[:0]
	for _, t := range ordered {
		if singular, ok := strings.CutSuffix(t, "s"); ok {
			if _, both := seen[singular]; both {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
