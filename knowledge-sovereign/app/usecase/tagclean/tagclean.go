// Package tagclean normalizes and filters auto-generated article tags before
// they are displayed on the trail or used for episode chaining. ML-generated
// tags arrive unmoderated (ADR-000182 lineage): stopwords, digit-only
// fragments, URL debris and case/plural variants all reach the read path, so
// display-bound tags pass through this pipeline instead of being surfaced raw.
package tagclean

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"knowledge-sovereign/driver/sovereign_db"
)

// junkWords holds exact-match tags that carry no filtering power, indexed from
// sovereign_db.JunkTagWords so SQL candidate ranking and Go tag normalization
// share a single source of truth.
var junkWords = buildJunkWordsSet(sovereign_db.JunkTagWords)

func buildJunkWordsSet(words []string) map[string]struct{} {
	set := make(map[string]struct{}, len(words))
	for _, w := range words {
		set[w] = struct{}{}
	}
	return set
}

// JunkWords returns a copy of the canonical lowercase junk-tag vocabulary.
func JunkWords() []string {
	return slices.Clone(sovereign_db.JunkTagWords)
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
