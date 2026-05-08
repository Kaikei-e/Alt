package knowledge_loop_projector

import (
	"encoding/json"
	"strings"
	"testing"
)

// decodedChangeSummary mirrors the decode shape that alt-backend's
// decodeChangeSummary uses, so this test verifies the JSONB blob the
// projector writes is parseable by the read side without a separate
// integration test.
type decodedChangeSummary struct {
	Summary               string   `json:"summary"`
	ChangedFields         []string `json:"changed_fields"`
	PreviousEntryKey      *string  `json:"previous_entry_key"`
	AddedPhrases          []string `json:"added_phrases"`
	RemovedPhrases        []string `json:"removed_phrases"`
	UnchangedPhrasesCount *uint32  `json:"unchanged_phrases_count"`
	AddedTags             []string `json:"added_tags"`
	RemovedTags           []string `json:"removed_tags"`
}

func unmarshalCS(t *testing.T, b []byte) decodedChangeSummary {
	t.Helper()
	var out decodedChangeSummary
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal change_summary JSON: %v", err)
	}
	return out
}

func TestBuildChangeSummaryJSON_Empty(t *testing.T) {
	t.Parallel()

	got := buildChangeSummaryJSON(changeSummaryPayload{})
	if got != nil {
		t.Errorf("expected nil for empty payload; got %s", got)
	}
}

func TestBuildChangeSummaryJSON_LegacyOnly(t *testing.T) {
	t.Parallel()

	prev := "entry:abc"
	in := changeSummaryPayload{
		summary:          "Newer version available.",
		changedFields:    []string{"summary", "tags"},
		previousEntryKey: prev,
	}
	got := buildChangeSummaryJSON(in)
	if got == nil {
		t.Fatal("expected non-nil JSON")
	}
	dec := unmarshalCS(t, got)
	// Phase 3: producer summary now carries an appended deterministic
	// update hint derived from changed_fields. Assert the producer text
	// is preserved as the prefix and the hint is appended after an em-dash.
	wantPrefix := "Newer version available — "
	if !strings.HasPrefix(dec.Summary, wantPrefix) {
		t.Errorf("Summary prefix = %q; want HasPrefix %q", dec.Summary, wantPrefix)
	}
	if !strings.Contains(dec.Summary, "re-read the lede") {
		t.Errorf("Summary missing summary-field hint clause: %q", dec.Summary)
	}
	if !strings.Contains(dec.Summary, "rethink which thread") {
		t.Errorf("Summary missing tags-field hint clause: %q", dec.Summary)
	}
	if len(dec.AddedPhrases) != 0 || len(dec.RemovedPhrases) != 0 {
		t.Errorf("expected no redline arrays; got %+v", dec)
	}
	if dec.PreviousEntryKey == nil || *dec.PreviousEntryKey != prev {
		t.Errorf("PreviousEntryKey = %v; want %q", dec.PreviousEntryKey, prev)
	}
}

// TestComposeChangeSummaryWithHint_Templates pins the deterministic update
// hint templates per changed_fields combination. Phase 3 acceptance: the
// "What to update" copy is reproject-stable and never depends on wall-clock
// or LLM output.
func TestComposeChangeSummaryWithHint_Templates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		summary    string
		changed    []string
		wantSubstr []string
		wantPrefix string
	}{
		{
			name:       "summary only",
			summary:    "Author rewrote the lede.",
			changed:    []string{"summary"},
			wantSubstr: []string{"re-read the lede"},
			wantPrefix: "Author rewrote the lede — ",
		},
		{
			name:       "tags only",
			summary:    "Tag set rotated.",
			changed:    []string{"tags"},
			wantSubstr: []string{"rethink which thread"},
			wantPrefix: "Tag set rotated — ",
		},
		{
			name:       "source only",
			summary:    "Canonical URL moved.",
			changed:    []string{"source"},
			wantSubstr: []string{"verify the canonical link"},
			wantPrefix: "Canonical URL moved — ",
		},
		{
			name:       "summary and tags joined by semicolon",
			summary:    "Both shifted.",
			changed:    []string{"summary", "tags"},
			wantSubstr: []string{"re-read the lede", "rethink which thread", "; "},
			wantPrefix: "Both shifted — ",
		},
		{
			name:       "no producer summary - hint stands alone capitalized",
			summary:    "",
			changed:    []string{"summary"},
			wantSubstr: []string{"Re-read the lede"},
			wantPrefix: "Re-read the lede",
		},
		{
			name:       "unknown field returns producer summary unchanged",
			summary:    "Author rewrote the lede.",
			changed:    []string{"some_unknown_field"},
			wantSubstr: []string{"Author rewrote the lede."},
			wantPrefix: "Author rewrote the lede.",
		},
		{
			name:       "case insensitive field matching",
			summary:    "x",
			changed:    []string{"SUMMARY", "Tags"},
			wantSubstr: []string{"re-read", "rethink"},
			wantPrefix: "x — ",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := composeChangeSummaryWithHint(tc.summary, tc.changed)
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Errorf("composeChangeSummaryWithHint(%q, %v) = %q; want prefix %q",
					tc.summary, tc.changed, got, tc.wantPrefix)
			}
			for _, sub := range tc.wantSubstr {
				if !strings.Contains(got, sub) {
					t.Errorf("missing substring %q in %q", sub, got)
				}
			}
		})
	}
}

// TestComposeChangeSummaryWithHint_BoundedLength pins that the composed
// summary never exceeds the FE-friendly cap, even with extreme inputs.
func TestComposeChangeSummaryWithHint_BoundedLength(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("Lorem ipsum dolor sit amet. ", 20)
	got := composeChangeSummaryWithHint(long, []string{"summary", "tags", "source"})
	if len(got) > changeSummaryUpdateHintMaxLen {
		t.Errorf("composed summary length %d exceeds cap %d", len(got), changeSummaryUpdateHintMaxLen)
	}
}

// TestComposeChangeSummaryWithHint_Determinism confirms repeated invocations
// with the same inputs produce byte-identical output (reproject-safety).
func TestComposeChangeSummaryWithHint_Determinism(t *testing.T) {
	t.Parallel()
	first := composeChangeSummaryWithHint("Author rewrote.", []string{"summary", "tags"})
	for i := range 5 {
		got := composeChangeSummaryWithHint("Author rewrote.", []string{"summary", "tags"})
		if got != first {
			t.Fatalf("invocation %d diverged: %q vs %q", i, got, first)
		}
	}
}

func TestBuildChangeSummaryJSON_RedlineFromExcerpts(t *testing.T) {
	t.Parallel()

	in := changeSummaryPayload{
		summary:        "Headline updated.",
		oldSummaryText: "The market opened high. Bonds were flat.",
		newSummaryText: "The market opened high. Energy stocks rallied.",
		oldTags:        []string{"finance", "bonds"},
		newTags:        []string{"finance", "energy"},
	}
	got := buildChangeSummaryJSON(in)
	if got == nil {
		t.Fatal("expected non-nil JSON")
	}
	dec := unmarshalCS(t, got)

	if len(dec.AddedPhrases) != 1 || dec.AddedPhrases[0] != "Energy stocks rallied." {
		t.Errorf("AddedPhrases = %v; want [Energy stocks rallied.]", dec.AddedPhrases)
	}
	if len(dec.RemovedPhrases) != 1 || dec.RemovedPhrases[0] != "Bonds were flat." {
		t.Errorf("RemovedPhrases = %v; want [Bonds were flat.]", dec.RemovedPhrases)
	}
	if dec.UnchangedPhrasesCount == nil || *dec.UnchangedPhrasesCount != 1 {
		t.Errorf("UnchangedPhrasesCount = %v; want 1", dec.UnchangedPhrasesCount)
	}
	if len(dec.AddedTags) != 1 || dec.AddedTags[0] != "energy" {
		t.Errorf("AddedTags = %v; want [energy]", dec.AddedTags)
	}
	if len(dec.RemovedTags) != 1 || dec.RemovedTags[0] != "bonds" {
		t.Errorf("RemovedTags = %v; want [bonds]", dec.RemovedTags)
	}
}

func TestBuildChangeSummaryJSON_PreviousExcerptOnlyStillCarriesContext(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"old_summary_version_id": "summary-v1",
		"new_summary_version_id": "summary-v2",
		"previous_summary_excerpt": "The first version said bonds were flat."
	}`)
	parsed := parseChangeSummaryPayload(raw)
	got := buildChangeSummaryJSON(parsed)
	if got == nil {
		t.Fatal("expected non-nil JSON for DB-shaped SummarySuperseded payload")
	}

	dec := unmarshalCS(t, got)
	// Phase 3: producer summary (derived here from previous excerpt) is now
	// suffixed with the deterministic update hint clause for the
	// changed_fields=["summary"] case.
	wantPrefix := "The first version said bonds were flat — "
	if !strings.HasPrefix(dec.Summary, wantPrefix) {
		t.Errorf("Summary = %q; want HasPrefix %q", dec.Summary, wantPrefix)
	}
	if !strings.Contains(dec.Summary, "re-read the lede") {
		t.Errorf("Summary missing summary-field hint: %q", dec.Summary)
	}
	if len(dec.ChangedFields) != 1 || dec.ChangedFields[0] != "summary" {
		t.Errorf("ChangedFields = %v; want [summary]", dec.ChangedFields)
	}
	if len(dec.AddedPhrases) != 0 || len(dec.RemovedPhrases) != 0 {
		t.Errorf("expected no redline arrays without the new excerpt; got %+v", dec)
	}
}

func TestParseChangeSummaryPayload_AcceptsAlternateKeys(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"old_summary_excerpt": "Old text.",
		"new_summary_excerpt": "New text.",
		"old_tags": ["a", "b"],
		"new_tags": ["a", "c"],
		"old_entry_key": "prev:xyz",
		"supersede_reason": "newer summary"
	}`)
	got := parseChangeSummaryPayload(raw)
	if got.oldSummaryText != "Old text." {
		t.Errorf("oldSummaryText = %q", got.oldSummaryText)
	}
	if got.newSummaryText != "New text." {
		t.Errorf("newSummaryText = %q", got.newSummaryText)
	}
	if got.previousEntryKey != "prev:xyz" {
		t.Errorf("previousEntryKey = %q", got.previousEntryKey)
	}
	if got.summary != "newer summary" {
		t.Errorf("summary = %q", got.summary)
	}
	if len(got.oldTags) != 2 || len(got.newTags) != 2 {
		t.Errorf("tag arrays not parsed: old=%v new=%v", got.oldTags, got.newTags)
	}
}

func TestParseChangeSummaryPayload_EmptyAndMalformed(t *testing.T) {
	t.Parallel()

	cases := [][]byte{
		nil,
		[]byte(""),
		[]byte("not json"),
		[]byte("{}"),
		[]byte(`{"old_tags": "not an array"}`),
	}
	for i, raw := range cases {
		got := parseChangeSummaryPayload(raw)
		if !got.isEmpty() {
			t.Errorf("case %d: expected empty payload; got %+v", i, got)
		}
	}
}
