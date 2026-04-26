package knowledge_loop_projector

import (
	"reflect"
	"testing"
)

func TestComputeChangeDiff_PhraseAddRemove(t *testing.T) {
	t.Parallel()

	in := DiffInput{
		OldSummaryText: "The market opened high. Tech stocks led gains. Bonds were flat.",
		NewSummaryText: "The market opened high. Tech stocks led gains. Energy stocks rallied late.",
	}
	got := computeChangeDiff(in)

	wantAdded := []string{"Energy stocks rallied late."}
	wantRemoved := []string{"Bonds were flat."}
	if !reflect.DeepEqual(got.AddedPhrases, wantAdded) {
		t.Errorf("AddedPhrases = %v; want %v", got.AddedPhrases, wantAdded)
	}
	if !reflect.DeepEqual(got.RemovedPhrases, wantRemoved) {
		t.Errorf("RemovedPhrases = %v; want %v", got.RemovedPhrases, wantRemoved)
	}
	if got.UnchangedPhrasesCount != 2 {
		t.Errorf("UnchangedPhrasesCount = %d; want 2", got.UnchangedPhrasesCount)
	}
}

func TestComputeChangeDiff_NoOverlap(t *testing.T) {
	t.Parallel()

	in := DiffInput{
		OldSummaryText: "Old A. Old B.",
		NewSummaryText: "New X. New Y.",
	}
	got := computeChangeDiff(in)
	if len(got.AddedPhrases) != 2 || len(got.RemovedPhrases) != 2 {
		t.Errorf("expected fully disjoint diff; got added=%v removed=%v", got.AddedPhrases, got.RemovedPhrases)
	}
	if got.UnchangedPhrasesCount != 0 {
		t.Errorf("UnchangedPhrasesCount = %d; want 0", got.UnchangedPhrasesCount)
	}
}

func TestComputeChangeDiff_IdenticalText(t *testing.T) {
	t.Parallel()

	text := "Sentence one. Sentence two."
	got := computeChangeDiff(DiffInput{OldSummaryText: text, NewSummaryText: text})
	if len(got.AddedPhrases) != 0 || len(got.RemovedPhrases) != 0 {
		t.Errorf("expected empty diff for identical text; got %+v", got)
	}
	if got.UnchangedPhrasesCount != 2 {
		t.Errorf("UnchangedPhrasesCount = %d; want 2", got.UnchangedPhrasesCount)
	}
}

func TestComputeChangeDiff_EmptyInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   DiffInput
	}{
		{name: "both empty", in: DiffInput{}},
		{name: "old empty, new present", in: DiffInput{NewSummaryText: "Only the new side."}},
		{name: "old present, new empty", in: DiffInput{OldSummaryText: "Only the old side."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeChangeDiff(tc.in)
			if got.UnchangedPhrasesCount != 0 {
				t.Errorf("Unchanged should be 0 for %s; got %d", tc.name, got.UnchangedPhrasesCount)
			}
		})
	}
}

func TestComputeChangeDiff_WhitespaceCollapse(t *testing.T) {
	t.Parallel()

	got := computeChangeDiff(DiffInput{
		OldSummaryText: "  A   sentence.\n\n   Another  one. ",
		NewSummaryText: "A sentence.\nAnother one.",
	})
	if len(got.AddedPhrases) != 0 || len(got.RemovedPhrases) != 0 {
		t.Errorf("whitespace differences should not produce a diff; got %+v", got)
	}
	if got.UnchangedPhrasesCount != 2 {
		t.Errorf("UnchangedPhrasesCount = %d; want 2", got.UnchangedPhrasesCount)
	}
}

func TestComputeChangeDiff_DedupePreservesOrder(t *testing.T) {
	t.Parallel()

	// New side has the same new sentence twice. Output should contain it
	// exactly once and preserve the input order — important so the redline
	// renders top-down without surprise reorder.
	got := computeChangeDiff(DiffInput{
		OldSummaryText: "Original.",
		NewSummaryText: "First new. First new. Second new.",
	})
	want := []string{"First new.", "Second new."}
	if !reflect.DeepEqual(got.AddedPhrases, want) {
		t.Errorf("AddedPhrases = %v; want %v", got.AddedPhrases, want)
	}
}

func TestComputeChangeDiff_TagsSetDiff(t *testing.T) {
	t.Parallel()

	got := computeChangeDiff(DiffInput{
		OldTags: []string{"finance", "markets", "Bonds"},
		NewTags: []string{"finance", "markets", "energy", "Renewables"},
	})
	wantAdded := []string{"Renewables", "energy"}
	wantRemoved := []string{"Bonds"}
	if !reflect.DeepEqual(got.AddedTags, wantAdded) {
		t.Errorf("AddedTags = %v; want %v", got.AddedTags, wantAdded)
	}
	if !reflect.DeepEqual(got.RemovedTags, wantRemoved) {
		t.Errorf("RemovedTags = %v; want %v", got.RemovedTags, wantRemoved)
	}
}

func TestComputeChangeDiff_TagsCaseInsensitive(t *testing.T) {
	t.Parallel()

	got := computeChangeDiff(DiffInput{
		OldTags: []string{"AI", "ml"},
		NewTags: []string{"ai", "ML"},
	})
	if len(got.AddedTags) != 0 || len(got.RemovedTags) != 0 {
		t.Errorf("case-only difference should not register; got added=%v removed=%v", got.AddedTags, got.RemovedTags)
	}
}

func TestComputeChangeDiff_TagsTrimAndIgnoreEmpty(t *testing.T) {
	t.Parallel()

	got := computeChangeDiff(DiffInput{
		OldTags: []string{"  finance ", "", "markets"},
		NewTags: []string{"finance", "  ", "energy"},
	})
	wantAdded := []string{"energy"}
	wantRemoved := []string{"markets"}
	if !reflect.DeepEqual(got.AddedTags, wantAdded) {
		t.Errorf("AddedTags = %v; want %v", got.AddedTags, wantAdded)
	}
	if !reflect.DeepEqual(got.RemovedTags, wantRemoved) {
		t.Errorf("RemovedTags = %v; want %v", got.RemovedTags, wantRemoved)
	}
}

// TestComputeChangeDiff_Determinism guards reproject-safety: replaying the
// same SummarySuperseded event must yield bit-identical change_summary
// JSON. If the function ever introduces map iteration in its output path
// without deterministic ordering, this test catches it.
func TestComputeChangeDiff_Determinism(t *testing.T) {
	t.Parallel()

	in := DiffInput{
		OldSummaryText: "A. B. C.",
		NewSummaryText: "A. D. E.",
		OldTags:        []string{"a", "b", "c"},
		NewTags:        []string{"a", "d", "e"},
	}
	first := computeChangeDiff(in)
	for i := 0; i < 200; i++ {
		next := computeChangeDiff(in)
		if !reflect.DeepEqual(next, first) {
			t.Fatalf("non-deterministic at iter %d: %+v vs %+v", i, first, next)
		}
	}
}
