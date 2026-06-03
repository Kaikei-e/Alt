package knowledge_loop_projector

import (
	"bytes"
	"reflect"
	"testing"
)

// The relation-set is persisted as JSONB opaque bytes on the entry (same idiom
// as surface_score_inputs). Enums serialize as stable strings so a reproject
// across an enum-value reshuffle stays bit-stable.

func TestMarshalRelations_EmptyIsNil(t *testing.T) {
	if got := marshalRelations(nil); got != nil {
		t.Fatalf("empty relation-set must marshal to nil (NULL column), got %q", got)
	}
}

func TestMarshalRelations_StableStringEnums(t *testing.T) {
	rels := []Relation{{
		Kind:      RelationKindContinuation,
		TargetRef: "a",
		Magnitude: 2,
		State:     RelationStateAdvancing,
		WhyText:   "w",
	}}
	got := marshalRelations(rels)
	if !bytes.Contains(got, []byte(`"kind":"continuation"`)) {
		t.Fatalf("kind must serialize as a stable string, got %s", got)
	}
	if !bytes.Contains(got, []byte(`"state":"advancing"`)) {
		t.Fatalf("state must serialize as a stable string, got %s", got)
	}
}

func TestRelations_RoundTrip(t *testing.T) {
	rels := []Relation{{
		Kind:      RelationKindContinuation,
		TargetRef: "article-1",
		Magnitude: 3,
		State:     RelationStateAdvanced,
		WhyText:   "x",
	}}
	back := parseRelations(marshalRelations(rels))
	if !reflect.DeepEqual(back, rels) {
		t.Fatalf("round-trip mismatch\n got: %#v\nwant: %#v", back, rels)
	}
}

func TestParseRelations_EmptyInputs(t *testing.T) {
	if got := parseRelations(nil); got != nil {
		t.Fatalf("nil bytes must parse to nil, got %#v", got)
	}
	if got := parseRelations([]byte("")); got != nil {
		t.Fatalf("empty bytes must parse to nil, got %#v", got)
	}
}
