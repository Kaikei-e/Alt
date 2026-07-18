// Package tagclean normalizes and filters auto-generated article tags before
// they are displayed on the trail or used for episode chaining. ML-generated
// tags arrive unmoderated (ADR-000182 lineage): stopwords, digit-only
// fragments, URL debris and case/plural variants all reach the read path, so
// display-bound tags pass through this pipeline instead of being surfaced raw.
package tagclean

// Normalize returns the canonical form of one tag, or "" when the tag is
// junk: an English/Japanese stopword, digit-only, shorter than two runes, or
// URL/HTML debris.
func Normalize(tag string) string {
	panic("not implemented")
}

// CleanDisplay normalizes every tag, drops junk, and merges duplicates —
// including case variants and naive English singular/plural pairs — while
// preserving first-seen order.
func CleanDisplay(tags []string) []string {
	panic("not implemented")
}
