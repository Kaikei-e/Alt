package knowledge_loop_projector

import (
	"encoding/json"
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
	if dec.Summary != in.summary {
		t.Errorf("Summary = %q; want %q", dec.Summary, in.summary)
	}
	if len(dec.AddedPhrases) != 0 || len(dec.RemovedPhrases) != 0 {
		t.Errorf("expected no redline arrays; got %+v", dec)
	}
	if dec.PreviousEntryKey == nil || *dec.PreviousEntryKey != prev {
		t.Errorf("PreviousEntryKey = %v; want %q", dec.PreviousEntryKey, prev)
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
	if dec.Summary != "The first version said bonds were flat." {
		t.Errorf("Summary = %q; want previous excerpt", dec.Summary)
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
