package trail_planner

import (
	"encoding/json"
	"fmt"
	"strings"
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

// anchorRef is the resolved spine anchor a branch's why points back to: the
// item it forks from, the title that names it, and the verb phrase that makes
// the claim true.
type anchorRef struct {
	itemKey   string
	title     string
	whyPhrase string
}

// whyPhraseByVerb renders each engagement verb as the subject-verb half of an
// anchored why. It is deliberately total over sovereign_db.EngagementVerbs and
// empty of everything else (§C4).
var whyPhraseByVerb = map[string]string{
	"read":     "you read",
	"asked":    "you asked about",
	"listened": "you listened to",
}

func whyPhraseForVerb(verb string) (string, bool) {
	phrase, ok := whyPhraseByVerb[verb]
	return phrase, ok
}

// buildClusterBranch turns a Cluster candidate into a fully-populated branch:
// a new item that situates into a topic the user already follows. The
// four-tuple is always set, and the why is anchored (D28(a)): it names the
// anchor item's title in quotes, phrased from the act that actually happened,
// never a generic "a topic you follow" claim with no concrete reference back
// to what the user did.
func buildClusterBranch(userID uuid.UUID, anchor anchorRef, c sovereign_db.TrailClusterCandidate) BranchProposedPayload {
	refs := make([]EvidenceRef, 0, len(c.SharedTags)+1)
	for _, tag := range c.SharedTags {
		refs = append(refs, EvidenceRef{RefID: tag, Label: tag, Kind: "tag"})
	}
	refs = append(refs, EvidenceRef{RefID: c.TargetItemKey, Label: c.TargetTitle, Kind: "article"})

	confidence := "plausible"
	if len(c.SharedTags) >= 2 {
		confidence = "corroborated"
	}

	return BranchProposedPayload{
		BranchKey:      "cluster:" + userID.String() + ":" + c.TargetItemKey,
		AnchorItemKey:  anchor.itemKey,
		RelationKind:   "cluster",
		Why:            fmt.Sprintf("Because %s %q — joins %s", anchor.whyPhrase, anchor.title, strings.Join(c.SharedTags, ", ")),
		EvidenceRefs:   refs,
		Confidence:     confidence,
		TargetItemKey:  c.TargetItemKey,
		TargetTitle:    c.TargetTitle,
		PlannerVersion: plannerVersion,
	}
}

// buildContinuationBranch turns a Continuation candidate into a fully-
// populated, self-referential branch (D27, Wave 11): the target IS the
// anchor — past engagement with the SAME item is what makes it continuation
// material, not a new item situating into a followed topic (contrast
// buildClusterBranch). The why is anchored on the candidate's own title,
// quoted, per the same D28(a) contract.
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
