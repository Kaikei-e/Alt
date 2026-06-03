package knowledge_loop

import (
	"testing"

	loopv1 "alt/gen/proto/alt/knowledge/loop/v1"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// ADR-000937: the sovereign relation-set JSONB is decoded into structured
// public Relation messages the FE renders as the Orient surface.

func TestDecodeRelations_Continuation(t *testing.T) {
	b := []byte(`[{"kind":"continuation","target_ref":"article-1","magnitude":2,"state":"advancing","why_text":"keep going"}]`)
	got := decodeRelations(b)
	if len(got) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(got))
	}
	r := got[0]
	if r.Kind != loopv1.RelationKind_RELATION_KIND_CONTINUATION {
		t.Errorf("kind: got %v, want CONTINUATION", r.Kind)
	}
	if r.State != loopv1.RelationState_RELATION_STATE_ADVANCING {
		t.Errorf("state: got %v, want ADVANCING", r.State)
	}
	if r.TargetRef != "article-1" || r.Magnitude != 2 || r.WhyText != "keep going" {
		t.Errorf("fields mismatch: %+v", r)
	}
}

func TestDecodeRelations_EmptyAndMalformed(t *testing.T) {
	if decodeRelations(nil) != nil {
		t.Error("nil bytes must decode to nil")
	}
	if decodeRelations([]byte("[]")) != nil {
		t.Error("empty array must decode to nil")
	}
	before := testutil.ToFloat64(relationsDecodeMalformedTotal)
	if decodeRelations([]byte("not json")) != nil {
		t.Error("malformed bytes must decode to nil")
	}
	if got := testutil.ToFloat64(relationsDecodeMalformedTotal); got != before+1 {
		t.Errorf("malformed decode must bump the fail-loud counter: got %v, want %v", got, before+1)
	}
}
