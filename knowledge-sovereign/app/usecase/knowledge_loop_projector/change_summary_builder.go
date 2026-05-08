package knowledge_loop_projector

import (
	"encoding/json"
	"strings"
)

// changeSummaryUpdateHintMaxLen caps the composed "what changed + what to
// update" sentence so the FE Changed card stays compact and the JSONB stays
// well under the 512-char why_text ceiling.
const changeSummaryUpdateHintMaxLen = 240

// updateHintForChangedFields returns a deterministic imperative hint
// describing what the user should do about the change, derived from the set
// of changed fields. Hint clauses are pinned constants — no LLM, no
// per-event variation — so reproject yields the same hint for the same
// changed_fields slice byte-for-byte.
//
// Returns empty string when no recognised field is in the input. The caller
// then leaves summary unchanged so an unfamiliar field set doesn't get
// arbitrary text appended.
func updateHintForChangedFields(changed []string) string {
	const (
		hintSummary = "re-read the lede before quoting"
		hintTags    = "rethink which thread this belongs to"
		hintSource  = "verify the canonical link"
	)
	set := make(map[string]struct{}, len(changed))
	for _, f := range changed {
		key := strings.ToLower(strings.TrimSpace(f))
		if key == "" {
			continue
		}
		set[key] = struct{}{}
	}
	parts := make([]string, 0, 3)
	if _, ok := set["summary"]; ok {
		parts = append(parts, hintSummary)
	}
	if _, ok := set["tags"]; ok {
		parts = append(parts, hintTags)
	}
	if _, ok := set["source"]; ok {
		parts = append(parts, hintSource)
	}
	return strings.Join(parts, "; ")
}

// composeChangeSummaryWithHint joins the producer's "what changed"
// description with the deterministic "what to update" hint via an em-dash.
// When the producer omits a summary the hint stands alone (capitalized,
// terminated). When changedFields produces no hint the producer summary is
// returned unchanged. Bounded length keeps the FE band compact.
func composeChangeSummaryWithHint(summary string, changedFields []string) string {
	hint := updateHintForChangedFields(changedFields)
	if hint == "" {
		return summary
	}
	trimmed := strings.TrimRight(summary, ". \n\t")
	var out string
	if trimmed == "" {
		out = strings.ToUpper(hint[:1]) + hint[1:] + "."
	} else {
		out = trimmed + " — " + hint + "."
	}
	if len(out) > changeSummaryUpdateHintMaxLen {
		out = strings.TrimRight(out[:changeSummaryUpdateHintMaxLen], " ")
	}
	return out
}

// buildChangeSummaryJSON assembles the JSONB blob stored in
// knowledge_loop_entries.change_summary from a Supersede event payload.
//
// Reproject-safety contract:
//   - Inputs are pulled exclusively from the event payload (excerpts and tag
//     arrays the upstream emitter included). The function never queries a
//     mutable view or the latest summary_versions row, so a replay against
//     the same event payload yields the same JSONB output bit-for-bit.
//   - When old and new excerpts are both present the redline arrays come
//     from computeChangeDiff (sentence-level set diff, deterministic).
//   - When the payload lacks one or both excerpts the redline arrays stay
//     empty / nil and the FE falls back to the legacy Then/Now diptych —
//     additive behavior preserved.
//
// Returned bytes are nil when there is nothing useful to record (the
// projector is then expected to leave entry.ChangeSummary as nil).
func buildChangeSummaryJSON(payload changeSummaryPayload) []byte {
	if payload.isEmpty() {
		return nil
	}

	type out struct {
		Summary               string   `json:"summary,omitempty"`
		ChangedFields         []string `json:"changed_fields,omitempty"`
		PreviousEntryKey      *string  `json:"previous_entry_key,omitempty"`
		AddedPhrases          []string `json:"added_phrases,omitempty"`
		RemovedPhrases        []string `json:"removed_phrases,omitempty"`
		UnchangedPhrasesCount *uint32  `json:"unchanged_phrases_count,omitempty"`
		AddedTags             []string `json:"added_tags,omitempty"`
		RemovedTags           []string `json:"removed_tags,omitempty"`
	}

	body := out{
		Summary:       composeChangeSummaryWithHint(payload.summary, payload.changedFields),
		ChangedFields: payload.changedFields,
	}
	if payload.previousEntryKey != "" {
		k := payload.previousEntryKey
		body.PreviousEntryKey = &k
	}

	if payload.canRedline() {
		diff := computeChangeDiff(DiffInput{
			OldSummaryText: payload.oldSummaryText,
			NewSummaryText: payload.newSummaryText,
			OldTags:        payload.oldTags,
			NewTags:        payload.newTags,
		})
		if len(diff.AddedPhrases) > 0 {
			body.AddedPhrases = diff.AddedPhrases
		}
		if len(diff.RemovedPhrases) > 0 {
			body.RemovedPhrases = diff.RemovedPhrases
		}
		if diff.UnchangedPhrasesCount > 0 {
			c := diff.UnchangedPhrasesCount
			body.UnchangedPhrasesCount = &c
		}
		if len(diff.AddedTags) > 0 {
			body.AddedTags = diff.AddedTags
		}
		if len(diff.RemovedTags) > 0 {
			body.RemovedTags = diff.RemovedTags
		}
	}

	b, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	return b
}

// changeSummaryPayload is the parsed Supersede event payload, containing
// only the fields the change_summary builder cares about. Filled by
// parseChangeSummaryPayload below.
type changeSummaryPayload struct {
	summary          string
	changedFields    []string
	previousEntryKey string
	oldSummaryText   string
	newSummaryText   string
	oldTags          []string
	newTags          []string
}

func (p changeSummaryPayload) isEmpty() bool {
	return p.summary == "" &&
		len(p.changedFields) == 0 &&
		p.previousEntryKey == "" &&
		!p.canRedline()
}

func (p changeSummaryPayload) canRedline() bool {
	hasSummary := p.oldSummaryText != "" && p.newSummaryText != ""
	hasTags := len(p.oldTags) > 0 || len(p.newTags) > 0
	return hasSummary || hasTags
}

func parseChangeSummaryPayload(raw json.RawMessage) changeSummaryPayload {
	out := changeSummaryPayload{}
	if len(raw) == 0 {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}

	out.summary = readString(m, "change_summary", "summary_message", "supersede_reason")
	out.previousEntryKey = readString(m, "previous_entry_key", "old_entry_key")
	out.oldSummaryText = readString(m,
		"previous_summary_excerpt", "old_summary_excerpt",
		"previous_summary_text", "old_summary_text")
	out.newSummaryText = readString(m,
		"summary_excerpt", "new_summary_excerpt",
		"summary_text", "new_summary_text")

	out.oldTags = readStringSlice(m, "previous_tags", "old_tags")
	out.newTags = readStringSlice(m, "tags", "new_tags")
	out.changedFields = readStringSlice(m, "changed_fields")

	if out.summary == "" && out.oldSummaryText != "" {
		out.summary = out.oldSummaryText
	}
	if len(out.changedFields) == 0 && (out.summary != "" || out.oldSummaryText != "" || out.newSummaryText != "") {
		out.changedFields = []string{"summary"}
	}

	return out
}

func readStringSlice(m map[string]any, keys ...string) []string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}
