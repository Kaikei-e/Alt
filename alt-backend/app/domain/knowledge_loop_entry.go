package domain

import (
	"time"

	"github.com/google/uuid"
)

// Knowledge Loop stage (OODA-style: Observe → Orient → Decide → Act).
type LoopStage string

const (
	LoopStageObserve LoopStage = "observe"
	LoopStageOrient  LoopStage = "orient"
	LoopStageDecide  LoopStage = "decide"
	LoopStageAct     LoopStage = "act"
)

// SurfaceBucket is the UI placement axis, independent of stage.
type SurfaceBucket string

const (
	SurfaceNow      SurfaceBucket = "now"
	SurfaceContinue SurfaceBucket = "continue"
	SurfaceChanged  SurfaceBucket = "changed"
	SurfaceReview   SurfaceBucket = "review"
)

// DismissState tracks active/deferred/dismissed/completed lifecycle.
type DismissState string

const (
	DismissActive    DismissState = "active"
	DismissDeferred  DismissState = "deferred"
	DismissDismissed DismissState = "dismissed"
	DismissCompleted DismissState = "completed"
)

// WhyKind is the structural categorization of a why payload.
// The projector maps legacy Phase-0 why codes (new_unread, in_weekly_recap, ...) into a WhyKind
// via an exhaustive mapping table versioned by WhyMappingVersion.
type WhyKind string

const (
	WhyKindSource  WhyKind = "source_why"
	WhyKindPattern WhyKind = "pattern_why"
	WhyKindRecall  WhyKind = "recall_why"
	WhyKindChange  WhyKind = "change_why"
)

// LoopPriority is the accessibility-facing priority token.
// UI layer maps to i18n labels; do not put display text in the domain.
type LoopPriority string

const (
	LoopPriorityCritical   LoopPriority = "critical"
	LoopPriorityContinuing LoopPriority = "continuing"
	LoopPriorityConfirm    LoopPriority = "confirm"
	LoopPriorityReference  LoopPriority = "reference"
)

// LoopServiceQuality describes overall Loop read-path health.
type LoopServiceQuality string

const (
	LoopQualityFull     LoopServiceQuality = "full"
	LoopQualityDegraded LoopServiceQuality = "degraded"
	LoopQualityFallback LoopServiceQuality = "fallback"
)

// RenderDepthHint is an abstract depth level; the view layer maps 1..4 to
// Z offset / shadow / saturation / brightness. Projection never returns px.
type RenderDepthHint int16

const (
	RenderDepthFlat     RenderDepthHint = 1
	RenderDepthLight    RenderDepthHint = 2
	RenderDepthStrong   RenderDepthHint = 3
	RenderDepthCritical RenderDepthHint = 4
)

// EvidenceRef is a single source-backed evidence pointer used in WhyPayload.
type EvidenceRef struct {
	RefID string `json:"ref_id"`
	Label string `json:"label,omitempty"`
}

// WhyPayload is the structured explanation for why an entry surfaced.
// Plain text only. Max 512 chars. No Markdown or HTML (UI renders as text).
type WhyPayload struct {
	Kind         WhyKind       `json:"kind"`
	Text         string        `json:"text"`
	Confidence   *float32      `json:"confidence,omitempty"`
	EvidenceRefs []EvidenceRef `json:"evidence_refs"`
}

// ArtifactVersionRef points to at least one versioned artifact.
// Server MUST reject if all three are nil (not expressible in proto3).
type ArtifactVersionRef struct {
	SummaryVersionID *string `json:"summary_version_id,omitempty"`
	TagSetVersionID  *string `json:"tag_set_version_id,omitempty"`
	LensVersionID    *string `json:"lens_version_id,omitempty"`
}

// KnowledgeLoopEntry is one unit of Loop proposal.
// A single article may spawn multiple entries; (user_id, lens_mode_id, entry_key) is the identity.
//
// projected_at is intentionally unexported — it is internal debug metadata and MUST NOT
// serialize into JSON / proto / metrics / production logs.
type KnowledgeLoopEntry struct {
	UserID        uuid.UUID `json:"user_id" db:"user_id"`
	TenantID      uuid.UUID `json:"tenant_id" db:"tenant_id"`
	LensModeID    string    `json:"lens_mode_id" db:"lens_mode_id"`
	EntryKey      string    `json:"entry_key" db:"entry_key"`
	SourceItemKey string    `json:"source_item_key" db:"source_item_key"`

	ProposedStage LoopStage     `json:"proposed_stage" db:"proposed_stage"`
	SurfaceBucket SurfaceBucket `json:"surface_bucket" db:"surface_bucket"`

	ProjectionRevision   int64 `json:"projection_revision" db:"projection_revision"`
	ProjectionSeqHiwater int64 `json:"projection_seq_hiwater" db:"projection_seq_hiwater"`
	SourceEventSeq       int64 `json:"source_event_seq" db:"source_event_seq"`

	FreshnessAt      time.Time  `json:"freshness_at" db:"freshness_at"`
	SourceObservedAt *time.Time `json:"source_observed_at,omitempty" db:"source_observed_at"`
	projectedAt      time.Time  // internal debug only; never serialize

	ArtifactVersionRef ArtifactVersionRef `json:"artifact_version_ref"`

	WhyKind           WhyKind       `json:"why_kind" db:"why_kind"`
	WhyText           string        `json:"why_text" db:"why_text"`
	WhyConfidence     *float32      `json:"why_confidence,omitempty" db:"why_confidence"`
	WhyEvidenceRefIDs []string      `json:"why_evidence_ref_ids" db:"why_evidence_ref_ids"`
	WhyEvidenceRefs   []EvidenceRef `json:"why_evidence_refs"`

	ChangeSummary   []byte `json:"change_summary,omitempty" db:"change_summary"`
	ContinueContext []byte `json:"continue_context,omitempty" db:"continue_context"`
	DecisionOptions []byte `json:"decision_options,omitempty" db:"decision_options"`
	ActTargets      []byte `json:"act_targets,omitempty" db:"act_targets"`

	SupersededByEntryKey *string      `json:"superseded_by_entry_key,omitempty" db:"superseded_by_entry_key"`
	DismissState         DismissState `json:"dismiss_state" db:"dismiss_state"`

	RenderDepthHint RenderDepthHint `json:"render_depth_hint" db:"render_depth_hint"`
	LoopPriority    LoopPriority    `json:"loop_priority" db:"loop_priority"`
}

// ProjectedAtForDebug returns the internal projected_at; callers MUST NOT expose this value.
// Use only in runbook-level diagnostic code paths under LOG_LEVEL=debug, non-production.
func (e KnowledgeLoopEntry) ProjectedAtForDebug() time.Time {
	return e.projectedAt
}

// SetProjectedAtForDriver is the only safe entry point to set projected_at from a driver scan.
// The private field prevents leak via json.Marshal / encoding/gob / proto serialization.
func (e *KnowledgeLoopEntry) SetProjectedAtForDriver(t time.Time) {
	e.projectedAt = t
}

// KnowledgeLoopSessionState holds per-user-per-lens session cursor.
// current_stage_entered_at MUST be derived from the triggering event's occurred_at.
type KnowledgeLoopSessionState struct {
	UserID     uuid.UUID `json:"user_id" db:"user_id"`
	TenantID   uuid.UUID `json:"tenant_id" db:"tenant_id"`
	LensModeID string    `json:"lens_mode_id" db:"lens_mode_id"`

	CurrentStage          LoopStage `json:"current_stage" db:"current_stage"`
	CurrentStageEnteredAt time.Time `json:"current_stage_entered_at" db:"current_stage_entered_at"`

	FocusedEntryKey    *string `json:"focused_entry_key,omitempty" db:"focused_entry_key"`
	ForegroundEntryKey *string `json:"foreground_entry_key,omitempty" db:"foreground_entry_key"`

	LastObservedEntryKey *string `json:"last_observed_entry_key,omitempty" db:"last_observed_entry_key"`
	LastOrientedEntryKey *string `json:"last_oriented_entry_key,omitempty" db:"last_oriented_entry_key"`
	LastDecidedEntryKey  *string `json:"last_decided_entry_key,omitempty" db:"last_decided_entry_key"`
	LastActedEntryKey    *string `json:"last_acted_entry_key,omitempty" db:"last_acted_entry_key"`
	LastReturnedEntryKey *string `json:"last_returned_entry_key,omitempty" db:"last_returned_entry_key"`
	LastDeferredEntryKey *string `json:"last_deferred_entry_key,omitempty" db:"last_deferred_entry_key"`

	ProjectionRevision   int64 `json:"projection_revision" db:"projection_revision"`
	ProjectionSeqHiwater int64 `json:"projection_seq_hiwater" db:"projection_seq_hiwater"`
	// Note: projected_at is not mirrored into alt-backend domain.
	// It is owned by knowledge-sovereign (sovereign_db) and never crosses the RPC boundary.
}

// KnowledgeLoopSurface describes a per-bucket surface summary.
type KnowledgeLoopSurface struct {
	UserID        uuid.UUID     `json:"user_id" db:"user_id"`
	TenantID      uuid.UUID     `json:"tenant_id" db:"tenant_id"`
	LensModeID    string        `json:"lens_mode_id" db:"lens_mode_id"`
	SurfaceBucket SurfaceBucket `json:"surface_bucket" db:"surface_bucket"`

	PrimaryEntryKey    *string  `json:"primary_entry_key,omitempty" db:"primary_entry_key"`
	SecondaryEntryKeys []string `json:"secondary_entry_keys" db:"secondary_entry_keys"`

	ProjectionRevision   int64     `json:"projection_revision" db:"projection_revision"`
	ProjectionSeqHiwater int64     `json:"projection_seq_hiwater" db:"projection_seq_hiwater"`
	FreshnessAt          time.Time `json:"freshness_at" db:"freshness_at"`
	// Note: projected_at is not mirrored into alt-backend domain.
	// It is owned by knowledge-sovereign (sovereign_db) and never crosses the RPC boundary.

	ServiceQuality LoopServiceQuality `json:"service_quality" db:"service_quality"`
	LoopHealth     []byte             `json:"loop_health,omitempty" db:"loop_health"`
}
