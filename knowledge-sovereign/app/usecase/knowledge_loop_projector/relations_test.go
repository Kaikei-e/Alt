package knowledge_loop_projector

import (
	"reflect"
	"testing"
)

// ADR-000937 first slice: extractRelations preserves the Continuation evidence
// as a first-class relation instead of collapsing it into a bucket. These
// tests pin the Relation shape and the OPEN→ADVANCING→ADVANCED state ladder
// that drives the visible return diff.

func TestExtractRelations_NoEvidence_EmptyRelationSet(t *testing.T) {
	got := extractRelations(SurfaceScoreInputs{EventType: EventArticleCreated})
	if len(got) != 0 {
		t.Fatalf("expected no relations for an entry with no continuation evidence, got %#v", got)
	}
}

func TestExtractRelations_OpenInteractionOnly_ContinuationOpen(t *testing.T) {
	got := extractRelations(SurfaceScoreInputs{
		HasOpenInteraction: true,
		ArticleID:          "article-1",
	})
	want := []Relation{{
		Kind:      RelationKindContinuation,
		TargetRef: "article-1",
		Magnitude: 1,
		State:     RelationStateOpen,
		WhyText:   continuationWhyText(RelationStateOpen, 1),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("open-interaction continuation mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestExtractRelations_OneContinueAction_Advancing(t *testing.T) {
	got := extractRelations(SurfaceScoreInputs{
		HasOpenInteraction:        true,
		RecentContinueActionCount: 1,
		ArticleID:                 "article-1",
	})
	if len(got) != 1 {
		t.Fatalf("expected exactly one continuation relation, got %#v", got)
	}
	if got[0].State != RelationStateAdvancing {
		t.Fatalf("one continue action must advance the thread to ADVANCING, got %v", got[0].State)
	}
}

func TestExtractRelations_RepeatedContinue_Advanced(t *testing.T) {
	got := extractRelations(SurfaceScoreInputs{
		RecentContinueActionCount: 2,
		ArticleID:                 "article-1",
	})
	if len(got) != 1 {
		t.Fatalf("expected exactly one continuation relation, got %#v", got)
	}
	if got[0].State != RelationStateAdvanced {
		t.Fatalf("two continue actions must mark the thread ADVANCED, got %v", got[0].State)
	}
}

func TestExtractRelations_AugurLinkAndQuestionScore_CountTowardMagnitude(t *testing.T) {
	got := extractRelations(SurfaceScoreInputs{
		HasAugurLink:              true,
		QuestionContinuationScore: 2,
		ArticleID:                 "article-1",
	})
	if len(got) != 1 {
		t.Fatalf("expected one continuation relation, got %#v", got)
	}
	// 1 (augur link) + 2 (question continuation) = 3 contacts, no continue acts
	// yet so the thread is still OPEN.
	if got[0].Magnitude != 3 {
		t.Fatalf("expected magnitude 3 from augur link + question score, got %d", got[0].Magnitude)
	}
	if got[0].State != RelationStateOpen {
		t.Fatalf("no continue action yet means OPEN, got %v", got[0].State)
	}
}

func TestExtractRelations_Pure_SameInputsSameOutput(t *testing.T) {
	in := SurfaceScoreInputs{HasOpenInteraction: true, RecentContinueActionCount: 1, ArticleID: "a"}
	first := extractRelations(in)
	second := extractRelations(in)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("extractRelations must be pure (reproject-safe): %#v != %#v", first, second)
	}
}
