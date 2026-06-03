package knowledge_loop_projector

import (
	"encoding/json"
	"fmt"
)

// Relation is a first-class, reproject-safe edge from an entry to the user's
// existing orientation (ADR-000937). Where decideBucketV2 collapses the rich
// SurfaceScoreInputs into a single bucket and discards the evidence, the
// relation-set preserves it: each Relation names HOW this entry situates
// against what the user already knows. The Orient surface renders the
// relation-set directly, and the loop is closed by showing a Relation's State
// transition on return.
//
// extractRelations is a pure function of SurfaceScoreInputs, exactly like
// decideBucketV2, so replaying the same event log reproduces the same
// relation-set bit-for-bit. No wall-clock, no latest-state queries.
type RelationKind int

const (
	RelationKindUnspecified RelationKind = iota
	// RelationKindContinuation: this entry extends a thread the user already
	// started (opened / asked / revisited / has an open Augur link). The
	// first relation kind wired end-to-end in the ADR-000937 first slice.
	RelationKindContinuation
	// Reserved for later slices — declared so the enum is stable across the
	// reproject boundary, but extractRelations does not emit them yet.
	RelationKindContradiction
	RelationKindCluster
	RelationKindInquiry
)

// RelationState is the lifecycle position of a relation, derived purely from
// the event-log evidence. It is the fuel for the visible "loop closed" return
// diff: a Continuation surfaces OPEN, advances to ADVANCING after the user
// returns once, and ADVANCED after repeated engagement.
type RelationState int

const (
	RelationStateUnspecified RelationState = iota
	RelationStateOpen
	RelationStateAdvancing
	RelationStateAdvanced
	RelationStateResolved
)

// Relation is the projected edge. TargetRef is a stable natural key (article
// id / entry key / versioned artifact id) so the edge is reproject-safe and
// dereferenceable without storing the anchor's mutable content. WhyText keeps
// the why-first-class invariant at the relation grain; the structured
// WhyPayload proto wiring lands when this slice reaches the proto phase.
type Relation struct {
	Kind      RelationKind
	TargetRef string
	Magnitude int32
	State     RelationState
	WhyText   string
}

// extractRelations turns the resolver's evidence into the relation-set without
// collapsing it. The first slice (ADR-000937) emits only the Continuation
// relation; later slices add Contradiction / Cluster / Inquiry from the same
// inputs.
func extractRelations(in SurfaceScoreInputs) []Relation {
	var out []Relation
	if rel, ok := continuationRelation(in); ok {
		out = append(out, rel)
	}
	return out
}

// continuationRelation derives the Continuation edge from the same evidence
// decideBucketV2 reads for its Continue branch — but it keeps the evidence
// instead of collapsing it. Contacts aggregate every continuation signal
// (open interaction, augur link, question-continuation, prior continue acts);
// the State ladder is driven solely by RecentContinueActionCount so the
// return diff reflects the user's own acts.
func continuationRelation(in SurfaceScoreInputs) (Relation, bool) {
	contacts := int32(in.RecentContinueActionCount)
	if in.HasOpenInteraction {
		contacts++
	}
	if in.HasAugurLink {
		contacts++
	}
	contacts += int32(in.QuestionContinuationScore)
	if contacts <= 0 {
		return Relation{}, false
	}

	state := RelationStateOpen
	switch {
	case in.RecentContinueActionCount >= 2:
		state = RelationStateAdvanced
	case in.RecentContinueActionCount == 1:
		state = RelationStateAdvancing
	}

	return Relation{
		Kind:      RelationKindContinuation,
		TargetRef: in.ArticleID,
		Magnitude: contacts,
		State:     state,
		WhyText:   continuationWhyText(state, contacts),
	}, true
}

// relationJSON is the on-disk shape of a Relation. Enums are stable strings so
// a reproject across an enum-value reshuffle stays bit-stable, matching the
// surface_score_inputs serialization idiom.
type relationJSON struct {
	Kind      string `json:"kind"`
	TargetRef string `json:"target_ref,omitempty"`
	Magnitude int32  `json:"magnitude,omitempty"`
	State     string `json:"state"`
	WhyText   string `json:"why_text,omitempty"`
}

// marshalRelations serializes the relation-set to JSONB opaque bytes. Empty
// sets marshal to nil so the column stays NULL rather than `[]`.
func marshalRelations(rels []Relation) []byte {
	if len(rels) == 0 {
		return nil
	}
	out := make([]relationJSON, 0, len(rels))
	for _, r := range rels {
		out = append(out, relationJSON{
			Kind:      relationKindString(r.Kind),
			TargetRef: r.TargetRef,
			Magnitude: r.Magnitude,
			State:     relationStateString(r.State),
			WhyText:   r.WhyText,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
}

// parseRelations reverses marshalRelations. nil / empty bytes parse to nil so
// reads of a NULL column yield no relations.
func parseRelations(raw []byte) []Relation {
	if len(raw) == 0 {
		return nil
	}
	var in []relationJSON
	if err := json.Unmarshal(raw, &in); err != nil || len(in) == 0 {
		return nil
	}
	out := make([]Relation, 0, len(in))
	for _, r := range in {
		out = append(out, Relation{
			Kind:      relationKindFromString(r.Kind),
			TargetRef: r.TargetRef,
			Magnitude: r.Magnitude,
			State:     relationStateFromString(r.State),
			WhyText:   r.WhyText,
		})
	}
	return out
}

func relationKindString(k RelationKind) string {
	switch k {
	case RelationKindContinuation:
		return "continuation"
	case RelationKindContradiction:
		return "contradiction"
	case RelationKindCluster:
		return "cluster"
	case RelationKindInquiry:
		return "inquiry"
	default:
		return "unspecified"
	}
}

func relationKindFromString(s string) RelationKind {
	switch s {
	case "continuation":
		return RelationKindContinuation
	case "contradiction":
		return RelationKindContradiction
	case "cluster":
		return RelationKindCluster
	case "inquiry":
		return RelationKindInquiry
	default:
		return RelationKindUnspecified
	}
}

func relationStateString(s RelationState) string {
	switch s {
	case RelationStateOpen:
		return "open"
	case RelationStateAdvancing:
		return "advancing"
	case RelationStateAdvanced:
		return "advanced"
	case RelationStateResolved:
		return "resolved"
	default:
		return "unspecified"
	}
}

func relationStateFromString(s string) RelationState {
	switch s {
	case "open":
		return RelationStateOpen
	case "advancing":
		return RelationStateAdvancing
	case "advanced":
		return RelationStateAdvanced
	case "resolved":
		return RelationStateResolved
	default:
		return RelationStateUnspecified
	}
}

// continuationWhyText renders the why at the relation grain. Plain text, no
// markup, keyed off the lifecycle state so the user reads the loop position in
// the relation itself.
func continuationWhyText(state RelationState, contacts int32) string {
	switch state {
	case RelationStateAdvancing:
		return "Continuing a thread you returned to — keep going."
	case RelationStateAdvanced:
		return fmt.Sprintf("A thread you have returned to %d times — your understanding is building.", contacts)
	default:
		return "Continuing a thread you opened but have not returned to."
	}
}
