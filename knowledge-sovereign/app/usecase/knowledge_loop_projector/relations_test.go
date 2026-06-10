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

func TestExtractRelations_AugurLinkAndQuestionScore_EmitsInquiry(t *testing.T) {
	// ADR-000938: augur / question signals feed Inquiry, not Continuation —
	// each signal maps to exactly one relation kind.
	got := extractRelations(SurfaceScoreInputs{
		HasAugurLink:              true,
		QuestionContinuationScore: 2,
		ArticleID:                 "article-1",
	})
	if len(got) != 1 {
		t.Fatalf("expected one inquiry relation, got %#v", got)
	}
	if got[0].Kind != RelationKindInquiry {
		t.Fatalf("augur link + question score must be an Inquiry, got kind %v", got[0].Kind)
	}
	// 1 (augur link) + 2 (question continuation) = 3.
	if got[0].Magnitude != 3 {
		t.Fatalf("expected magnitude 3 from augur link + question score, got %d", got[0].Magnitude)
	}
}

func TestExtractRelations_Contradiction_OpenAdvancingResolved(t *testing.T) {
	// ADR-000938: the changed/contradicted entry — which the Continuation-only
	// slice left empty — now surfaces a Contradiction relation, and its State
	// is the visible return diff driven purely by the event log.
	open := extractRelations(SurfaceScoreInputs{VersionDriftCount: 1, ArticleID: "a"})
	if len(open) != 1 || open[0].Kind != RelationKindContradiction || open[0].State != RelationStateOpen {
		t.Fatalf("version drift must surface a Contradiction OPEN, got %#v", open)
	}

	advancing := extractRelations(SurfaceScoreInputs{ContradictionCount: 1, CompareActionCount: 1, ArticleID: "a"})
	if len(advancing) != 1 || advancing[0].State != RelationStateAdvancing {
		t.Fatalf("a compare act must advance the Contradiction to ADVANCING, got %#v", advancing)
	}

	resolved := extractRelations(SurfaceScoreInputs{VersionDriftCount: 1, CompareActionCount: 1, AcceptedChangeCount: 1, ArticleID: "a"})
	if len(resolved) != 1 || resolved[0].State != RelationStateResolved {
		t.Fatalf("an accepted_change outcome must close the loop to RESOLVED, got %#v", resolved)
	}
}

func TestExtractRelations_Cluster_SituatesFreshEntry(t *testing.T) {
	// The dominant Orient case: a brand-new article with no self-referential
	// history still situates against the user's tracked topics via Cluster.
	got := extractRelations(SurfaceScoreInputs{TopicOverlapCount: 2, EventType: EventArticleCreated, ArticleID: "a"})
	if len(got) != 1 || got[0].Kind != RelationKindCluster {
		t.Fatalf("topic overlap on a fresh article must surface a Cluster relation, got %#v", got)
	}
}

func TestExtractRelations_AllKinds_NotCollapsed(t *testing.T) {
	// The anti-collapse guard: when the resolver computes fuel for several
	// kinds at once, extractRelations keeps ALL of them instead of picking one.
	got := extractRelations(SurfaceScoreInputs{
		VersionDriftCount:  1,    // Contradiction
		HasOpenInteraction: true, // Continuation
		TopicOverlapCount:  1,    // Cluster
		HasAugurLink:       true, // Inquiry
		ArticleID:          "a",
	})
	kinds := map[RelationKind]bool{}
	for _, r := range got {
		kinds[r.Kind] = true
	}
	for _, want := range []RelationKind{RelationKindContradiction, RelationKindContinuation, RelationKindCluster, RelationKindInquiry} {
		if !kinds[want] {
			t.Fatalf("expected all four relation kinds, missing %v in %#v", want, got)
		}
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
