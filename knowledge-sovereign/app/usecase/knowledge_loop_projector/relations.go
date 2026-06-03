package knowledge_loop_projector

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
	// returned to (opened / revisited / a continue act). Self-referential.
	RelationKindContinuation
	// RelationKindContradiction: this entry conflicts with a version or view
	// the user previously held (version drift / supersede). The relation kind
	// whose loop ADR-000938 closes end to end — its State carries the visible
	// return diff OPEN → ADVANCING → RESOLVED.
	RelationKindContradiction
	// RelationKindCluster: this (possibly brand-new) entry joins a topic / tag
	// group the user is already tracking. NOT self-referential, so it situates
	// a freshly ingested `now` article against the user's orientation.
	RelationKindCluster
	// RelationKindInquiry: this entry answers a question the user put to Augur.
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
// collapsing it. ADR-000938: it emits EVERY relation kind for which the
// resolver computed fuel — Contradiction, Continuation, Cluster, Inquiry — so
// the rich SurfaceScoreInputs are no longer thrown away one room over from
// decideBucketV2. Each signal feeds exactly one kind so a single piece of
// evidence cannot double-emit. Contradiction leads because it is the kind
// whose return diff this slice closes end to end.
func extractRelations(in SurfaceScoreInputs) []Relation {
	var out []Relation
	if rel, ok := contradictionRelation(in); ok {
		out = append(out, rel)
	}
	if rel, ok := continuationRelation(in); ok {
		out = append(out, rel)
	}
	if rel, ok := clusterRelation(in); ok {
		out = append(out, rel)
	}
	if rel, ok := inquiryRelation(in); ok {
		out = append(out, rel)
	}
	return out
}

// contradictionRelation derives the Contradiction edge: a newer version or a
// supersede contradicts the view the user previously held. The State carries
// the visible return diff and is a pure function of the event log:
//   - OPEN: drift exists, the user has not reconciled it.
//   - ADVANCING: the user compared the redline (compare act) but no
//     accepted_change outcome has landed yet.
//   - RESOLVED: an accepted_change outcome ("compare → dismiss = reconciled",
//     ADR-000908) means the user reconciled the contradiction.
func contradictionRelation(in SurfaceScoreInputs) (Relation, bool) {
	drift := int32(in.VersionDriftCount) + int32(in.ContradictionCount)
	if drift <= 0 {
		return Relation{}, false
	}
	state := RelationStateOpen
	switch {
	case in.AcceptedChangeCount > 0:
		state = RelationStateResolved
	case in.CompareActionCount > 0:
		state = RelationStateAdvancing
	}
	return Relation{
		Kind:      RelationKindContradiction,
		TargetRef: in.ArticleID,
		Magnitude: drift,
		State:     state,
		WhyText:   contradictionWhyText(state),
	}, true
}

// continuationRelation derives the Continuation edge — a thread the user
// returned to. Self-referential: only open interaction + prior continue acts
// on this same entry count. Augur / question signals feed Inquiry instead, so
// each signal maps to exactly one relation kind. The State ladder is driven
// solely by RecentContinueActionCount so the return diff reflects the user's
// own acts.
func continuationRelation(in SurfaceScoreInputs) (Relation, bool) {
	contacts := int32(in.RecentContinueActionCount)
	if in.HasOpenInteraction {
		contacts++
	}
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

// clusterRelation derives the Cluster edge — this entry joins a topic / tag
// group the user is already tracking. Unlike Continuation / Inquiry it is not
// self-referential, so it can situate a freshly ingested `now` article (the
// dominant Orient case the Continuation-only slice left empty). Overlap
// aggregates recap topic overlap, trending tag overlap, and cluster momentum.
func clusterRelation(in SurfaceScoreInputs) (Relation, bool) {
	overlap := int32(in.TopicOverlapCount) + int32(in.TagOverlapCount) + int32(in.RecapClusterMomentum)
	if overlap <= 0 {
		return Relation{}, false
	}
	state := RelationStateOpen
	if overlap >= 3 {
		state = RelationStateAdvancing
	}
	return Relation{
		Kind:      RelationKindCluster,
		TargetRef: in.ArticleID,
		Magnitude: overlap,
		State:     state,
		WhyText:   clusterWhyText(overlap),
	}, true
}

// inquiryRelation derives the Inquiry edge — this entry answers a question the
// user put to Augur. Self-referential: needs a prior conversation linked to
// this entry. Magnitude counts the linked questions.
func inquiryRelation(in SurfaceScoreInputs) (Relation, bool) {
	score := int32(in.QuestionContinuationScore)
	if in.HasAugurLink {
		score++
	}
	if score <= 0 {
		return Relation{}, false
	}
	return Relation{
		Kind:      RelationKindInquiry,
		TargetRef: in.ArticleID,
		Magnitude: score,
		State:     RelationStateOpen,
		WhyText:   inquiryWhyText(score),
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
		// ADR-000938 fail-loud: we just built these Relation structs, so a
		// marshal failure is a programming bug, not a "no fuel" empty set.
		// Silently returning nil would empty the Orient surface while looking
		// like a legitimately relation-less entry (the PM-2026-045 /
		// ADR-000928 silent-fallback failure mode). Panic instead.
		panic(fmt.Sprintf("knowledge_loop_projector: marshalRelations failed on built relations (bug): %v", err))
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
	if err := json.Unmarshal(raw, &in); err != nil {
		// ADR-000938 fail-loud: non-empty bytes that do not parse are a
		// data-quality bug (schema drift / corruption), not a legitimate empty
		// set. Empty / NULL columns hit the len(raw)==0 guard above and return
		// nil quietly; a malformed payload must be loud so it cannot masquerade
		// as "no relations".
		slog.Error("knowledge_loop_projector: parseRelations got malformed relation JSONB",
			"err", err, "bytes", len(raw))
		return nil
	}
	if len(in) == 0 {
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

// contradictionWhyText renders the why for the Contradiction edge keyed off the
// loop position, so the user reads where they are in reconciling the change.
func contradictionWhyText(state RelationState) string {
	switch state {
	case RelationStateResolved:
		return "You reconciled this — the contradiction with your earlier version is resolved."
	case RelationStateAdvancing:
		return "You compared the change — reconcile it to close the loop."
	default:
		return "A newer version contradicts what you read before — compare to reconcile."
	}
}

// clusterWhyText renders the why for the Cluster edge — how strongly this entry
// joins topics the user already tracks.
func clusterWhyText(overlap int32) string {
	if overlap >= 3 {
		return fmt.Sprintf("Joins %d threads of a topic you are actively following.", overlap)
	}
	return "Connects to a topic you have been following."
}

// inquiryWhyText renders the why for the Inquiry edge — this entry speaks to a
// question the user asked Augur.
func inquiryWhyText(score int32) string {
	if score >= 2 {
		return "Speaks to questions you have been asking Augur."
	}
	return "Speaks to a question you asked Augur."
}
