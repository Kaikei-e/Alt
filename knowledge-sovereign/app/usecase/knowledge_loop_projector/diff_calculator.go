package knowledge_loop_projector

import (
	"regexp"
	"sort"
	"strings"
)

// ChangeDiff is the structured output of computeChangeDiff. It mirrors the
// additive fields the proto's ChangeSummary message gained for redline-proof
// rendering, but the function itself does NOT depend on proto types — it
// works on plain strings so it can be unit tested without sovereignv1
// scaffolding and reused for log lines, ADR diffs, etc.
type ChangeDiff struct {
	AddedPhrases          []string
	RemovedPhrases        []string
	UnchangedPhrasesCount uint32
	AddedTags             []string
	RemovedTags           []string
}

// DiffInput is the pair of versioned-resource projections fed to the diff.
// Both old and new must be drawn from immutable artefacts (summary_versions
// rows by id, tag_set_versions rows by id) so that the same input pair
// always yields the same diff — preserving reproject-safety.
type DiffInput struct {
	OldSummaryText string
	NewSummaryText string
	OldTags        []string
	NewTags        []string
}

// computeChangeDiff produces the redline view of a summary supersede event.
//
// Phrase semantics are sentence-level: the function splits each summary on
// terminating punctuation + whitespace (`.`, `!`, `?`, newline) and
// compares the resulting sentence sets. Sentence-level diff is the right
// granularity for "Summary changed" narrative — word-level diff produces
// noisy redlines on stylistic edits that don't change meaning.
//
// Tag semantics are pure set diff with case-insensitive matching against
// trimmed forms; the original casing of the new side is preserved.
//
// Pure function, no time / network / mutable state. Replays of the same
// input are bit-identical.
func computeChangeDiff(in DiffInput) ChangeDiff {
	oldPhrases := splitSentences(in.OldSummaryText)
	newPhrases := splitSentences(in.NewSummaryText)

	oldSet := make(map[string]struct{}, len(oldPhrases))
	for _, p := range oldPhrases {
		oldSet[p] = struct{}{}
	}
	newSet := make(map[string]struct{}, len(newPhrases))
	for _, p := range newPhrases {
		newSet[p] = struct{}{}
	}

	added := make([]string, 0)
	for _, p := range newPhrases {
		if _, ok := oldSet[p]; !ok {
			added = append(added, p)
		}
	}
	removed := make([]string, 0)
	for _, p := range oldPhrases {
		if _, ok := newSet[p]; !ok {
			removed = append(removed, p)
		}
	}
	var unchanged uint32
	for _, p := range newPhrases {
		if _, ok := oldSet[p]; ok {
			unchanged++
		}
	}

	addedTags, removedTags := diffTags(in.OldTags, in.NewTags)

	return ChangeDiff{
		AddedPhrases:          dedupePreserveOrder(added),
		RemovedPhrases:        dedupePreserveOrder(removed),
		UnchangedPhrasesCount: unchanged,
		AddedTags:             addedTags,
		RemovedTags:           removedTags,
	}
}

// sentenceSplitRe matches a sentence terminator (`.!?`) followed by
// whitespace, OR a literal newline. Capturing the punctuation lets us keep
// the terminator with the sentence on the left side of the split.
var sentenceSplitRe = regexp.MustCompile(`(?m)([.!?])\s+|\n+`)

func splitSentences(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	// Replace each sentence-ending punctuation+space pair with the
	// punctuation followed by a sentinel newline; then split on \n+.
	normalized := sentenceSplitRe.ReplaceAllString(trimmed, "$1\n")
	raw := strings.Split(normalized, "\n")
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		s := strings.TrimSpace(r)
		s = strings.Join(strings.Fields(s), " ")
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func diffTags(oldTags, newTags []string) (added, removed []string) {
	oldNorm := make(map[string]string, len(oldTags))
	for _, t := range oldTags {
		k := strings.ToLower(strings.TrimSpace(t))
		if k == "" {
			continue
		}
		oldNorm[k] = strings.TrimSpace(t)
	}
	newNorm := make(map[string]string, len(newTags))
	for _, t := range newTags {
		k := strings.ToLower(strings.TrimSpace(t))
		if k == "" {
			continue
		}
		newNorm[k] = strings.TrimSpace(t)
	}
	added = make([]string, 0)
	for k, original := range newNorm {
		if _, ok := oldNorm[k]; !ok {
			added = append(added, original)
		}
	}
	removed = make([]string, 0)
	for k, original := range oldNorm {
		if _, ok := newNorm[k]; !ok {
			removed = append(removed, original)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func dedupePreserveOrder(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
