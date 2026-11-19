package trail_planner

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
)

// EventTrailBranchProposed is the system-only branch proposal event type.
const EventTrailBranchProposed = "trail.branch_proposed.v1"

// EventTrailBranchResolved is the user-action event recording how a branch was
// resolved (taken or dismissed).
const EventTrailBranchResolved = "trail.branch_resolved.v1"

// BranchResolvedPayload is the trail.branch_resolved.v1 event body.
// DismissReason is the optional one-tap scrutability signal (D28(d)): a
// non-empty value only ever accompanies resolution=="dismissed".
type BranchResolvedPayload struct {
	BranchKey     string `json:"branch_key"`
	Resolution    string `json:"resolution"` // "taken" | "dismissed"
	DismissReason string `json:"dismiss_reason,omitempty"`
}

// ValidResolution reports whether r is an accepted branch resolution.
func ValidResolution(r string) bool {
	return r == "taken" || r == "dismissed"
}

const plannerVersion = "v1"

// EvidenceRef mirrors the read-model evidence shape.
type EvidenceRef struct {
	RefID string `json:"ref_id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

// BranchProposedPayload is the trail.branch_proposed.v1 event body. The
// four-tuple is mandatory; Valid() is the contract gate the planner and the
// projector both apply.
type BranchProposedPayload struct {
	BranchKey      string        `json:"branch_key"`
	AnchorItemKey  string        `json:"anchor_item_key"`
	RelationKind   string        `json:"relation_kind"`
	Why            string        `json:"why"`
	EvidenceRefs   []EvidenceRef `json:"evidence_refs"`
	Confidence     string        `json:"confidence"`
	TargetItemKey  string        `json:"target_item_key"`
	TargetTitle    string        `json:"target_title"`
	PlannerVersion string        `json:"planner_version"`
}

// Valid reports whether the branch carries the full four-tuple. A branch that is
// not Valid must never be surfaced.
func (p BranchProposedPayload) Valid() bool {
	return p.RelationKind != "" && p.Why != "" && len(p.EvidenceRefs) > 0 && p.Confidence != ""
}

// whyPhraseByVerb renders each engagement verb as the subject-verb half of an
// anchored why. It is deliberately total over sovereign_db.EngagementVerbs and
// empty of everything else.
var whyPhraseByVerb = map[string]string{
	"read":     "you read",
	"asked":    "you asked about",
	"listened": "you listened to",
}

func whyPhraseForVerb(verb string) (string, bool) {
	phrase, ok := whyPhraseByVerb[verb]
	return phrase, ok
}

// buildContinuationBranch turns a Continuation candidate into a fully-
// populated, self-referential branch: the target IS the anchor — past
// engagement with the same item is what makes it continuation material. The why
// is anchored on the candidate's own title in quotes.
func buildContinuationBranch(userID uuid.UUID, whyPhrase string, c sovereign_db.TrailContinuationCandidate) BranchProposedPayload {
	return BranchProposedPayload{
		BranchKey:     "continuation:" + userID.String() + ":" + c.TargetItemKey,
		AnchorItemKey: c.TargetItemKey,
		RelationKind:  "continuation",
		Why:           fmt.Sprintf("Because %s %q and the thread went quiet — pick it back up.", whyPhrase, c.TargetTitle),
		EvidenceRefs: []EvidenceRef{
			{RefID: c.TargetItemKey, Label: c.TargetTitle, Kind: "article"},
		},
		Confidence:     "plausible",
		TargetItemKey:  c.TargetItemKey,
		TargetTitle:    c.TargetTitle,
		PlannerVersion: plannerVersion,
	}
}

// buildBranchProposedEvent is a pure builder that constructs the sovereign_db.KnowledgeEvent
// from its inputs, taking the occurrence timestamp as an explicit parameter.
func buildBranchProposedEvent(userID, tenantID uuid.UUID, payload BranchProposedPayload, occurredAt time.Time) (sovereign_db.KnowledgeEvent, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return sovereign_db.KnowledgeEvent{}, fmt.Errorf("trail_planner marshal: %w", err)
	}
	uid := userID
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    occurredAt,
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     "system",
		ActorID:       "trail-planner",
		EventType:     EventTrailBranchProposed,
		AggregateType: "trail_branch",
		AggregateID:   payload.BranchKey,
		DedupeKey:     EventTrailBranchProposed + ":" + payload.BranchKey,
		Payload:       body,
	}, nil
}
