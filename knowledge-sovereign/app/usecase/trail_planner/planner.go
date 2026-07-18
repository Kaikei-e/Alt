// Package trail_planner is the Knowledge Trail branch producer. It reads the
// current spine (footprints) and candidate items to derive typed branches and
// emits trail.branch_proposed.v1 events. It is the ONLY emitter of those events
// (system-only). As a producer it may read current state to decide what to emit;
// the projector that folds the resulting events stays payload-only (reproject-safe).
//
// Every emitted branch carries the four-tuple — relation_kind, why, evidence_refs,
// confidence — or it is not emitted. Untyped branches (the Loop decorated-feed
// failure) are impossible by construction.
package trail_planner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
// non-empty value only ever accompanies resolution=="dismissed". It is not
// new event vocabulary — the payload shape absorbs it — and the projector
// folds the event the same way regardless of whether it is present; planner
// calibration off this field is explicitly out of scope (D21).
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

// Repository is the narrow surface the planner needs.
type Repository interface {
	ListDistinctUserIDs(ctx context.Context) ([]uuid.UUID, error)
	GetLatestFootprintAnchor(ctx context.Context, userID uuid.UUID) (string, uuid.UUID, bool, error)
	// GetItemTitle resolves the anchor's display title (D28 — anchored why):
	// a branch's why must reference it, so an unresolvable title suppresses
	// emission rather than falling back to a generic why.
	GetItemTitle(ctx context.Context, userID uuid.UUID, itemKey string) (string, bool, error)
	DeriveTrailClusterCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]sovereign_db.TrailClusterCandidate, error)
	// DeriveTrailContinuationCandidates finds a thread the user already
	// engaged that has gone quiet without going deep (Wave 11, D27) — the
	// self-referential raw material for a Continuation branch.
	DeriveTrailContinuationCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]sovereign_db.TrailContinuationCandidate, error)
	AppendKnowledgeEvent(ctx context.Context, event sovereign_db.KnowledgeEvent) (int64, error)
}

// Config tunes the planner.
type Config struct {
	MaxBranchesPerUser int
	// Clock is injected so the emitted occurred_at is testable and the planner
	// holds no time.Now literal. Production wires time.Now.
	Clock func() time.Time
}

// Planner derives and emits branch proposals.
type Planner struct {
	repo   Repository
	logger *slog.Logger
	cfg    Config
}

func NewPlanner(repo Repository, logger *slog.Logger, cfg Config) *Planner {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.MaxBranchesPerUser <= 0 {
		cfg.MaxBranchesPerUser = 5
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &Planner{repo: repo, logger: logger, cfg: cfg}
}

// RunBatch derives Cluster branches for every user with a spine and emits a
// branch_proposed event per fresh candidate (idempotent via dedupe_key).
func (p *Planner) RunBatch(ctx context.Context) error {
	// Rule 8: a planner reached with no repository is a wiring bug — fail loud,
	// never silently no-op. This is business code, not a defensive nil-guard.
	if p.repo == nil {
		panic("trail_planner: repository not wired")
	}

	users, err := p.repo.ListDistinctUserIDs(ctx)
	if err != nil {
		return fmt.Errorf("trail_planner list users: %w", err)
	}
	var userErrs int
	for _, userID := range users {
		if err := p.planUser(ctx, userID); err != nil {
			userErrs++
			p.logger.ErrorContext(ctx, "trail_planner: user batch failed; continuing",
				slog.String("user_id", userID.String()),
				slog.String("error", err.Error()))
		}
	}
	if userErrs > 0 {
		p.logger.WarnContext(ctx, "trail_planner: batch completed with user errors",
			slog.Int("failed_users", userErrs),
			slog.Int("total_users", len(users)))
	}
	return nil
}

func (p *Planner) planUser(ctx context.Context, userID uuid.UUID) error {
	anchor, tenantID, ok, err := p.repo.GetLatestFootprintAnchor(ctx, userID)
	if err != nil {
		return fmt.Errorf("trail_planner anchor: %w", err)
	}
	if !ok {
		return nil // no footprints yet → nothing to anchor a branch on
	}

	// D28(a): a branch whose why does not reference its anchor is forbidden.
	// The anchor's title is that reference, so an unresolvable title must
	// suppress emission for this user — never fall back to a generic why.
	anchorTitle, titleOK, err := p.repo.GetItemTitle(ctx, userID, anchor)
	if err != nil {
		return fmt.Errorf("trail_planner anchor title: %w", err)
	}
	if !titleOK {
		p.logger.WarnContext(ctx, "trail.branch_anchor_unresolved",
			slog.String("user_id", userID.String()),
			slog.String("anchor_item_key", anchor))
		return nil
	}

	candidates, err := p.repo.DeriveTrailClusterCandidates(ctx, userID, p.cfg.MaxBranchesPerUser)
	if err != nil {
		return fmt.Errorf("trail_planner candidates: %w", err)
	}
	for _, c := range candidates {
		// A target the user cannot even read the name of is not a useful
		// proposal — surfacing it would render as a bare item key. Title-less
		// targets (upstream knowledge_home_items.title gaps) are skipped, not
		// proposed. The read path still applies a display fallback for any
		// title-less branches already in the log.
		if strings.TrimSpace(c.TargetTitle) == "" {
			continue
		}
		payload := buildClusterBranch(userID, anchor, anchorTitle, c)
		if err := p.emitBranch(ctx, userID, tenantID, payload); err != nil {
			return err
		}
	}

	if err := p.planContinuationBranch(ctx, userID, tenantID); err != nil {
		return err
	}
	return nil
}

// planContinuationBranch derives at most ONE Continuation candidate per user
// per run (D28 — 少数精鋭, precision over recall) and emits it. Continuation
// is self-referential (D27, contexts/knowledge-trail.md): the target IS the
// anchor, because past engagement with the SAME item — not tag overlap with a
// new item — is what qualifies it (contrast the Cluster loop above).
func (p *Planner) planContinuationBranch(ctx context.Context, userID, tenantID uuid.UUID) error {
	candidates, err := p.repo.DeriveTrailContinuationCandidates(ctx, userID, 1)
	if err != nil {
		return fmt.Errorf("trail_planner continuation candidates: %w", err)
	}
	if len(candidates) == 0 {
		return nil
	}
	// Defense in depth: cap to one regardless of what the repository returns —
	// a handful of quiet threads in the log must not fan out into several
	// proposals in one run.
	c := candidates[0]
	if strings.TrimSpace(c.TargetTitle) == "" {
		p.logger.WarnContext(ctx, "trail.branch_anchor_unresolved",
			slog.String("user_id", userID.String()),
			slog.String("anchor_item_key", c.TargetItemKey))
		return nil
	}
	payload := buildContinuationBranch(userID, c)
	return p.emitBranch(ctx, userID, tenantID, payload)
}

// emitBranch appends a validated branch_proposed event and logs its relation
// kind (Wave 11 observability — makes the type distribution across Cluster /
// Continuation / future kinds measurable). A branch that fails Valid() is
// dropped rather than emitted incomplete — untyped branches are forbidden by
// construction, not just by convention.
func (p *Planner) emitBranch(ctx context.Context, userID, tenantID uuid.UUID, payload BranchProposedPayload) error {
	if !payload.Valid() {
		p.logger.WarnContext(ctx, "trail_planner: dropping incomplete branch",
			slog.String("branch_key", payload.BranchKey))
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("trail_planner marshal: %w", err)
	}
	uid := userID
	evt := sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    p.cfg.Clock(),
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     "system",
		ActorID:       "trail-planner",
		EventType:     EventTrailBranchProposed,
		AggregateType: "trail_branch",
		AggregateID:   payload.BranchKey,
		DedupeKey:     EventTrailBranchProposed + ":" + payload.BranchKey,
		Payload:       body,
	}
	if _, err := p.repo.AppendKnowledgeEvent(ctx, evt); err != nil {
		return fmt.Errorf("trail_planner emit: %w", err)
	}
	p.logger.InfoContext(ctx, "trail.branch_proposed",
		slog.String("user_id", userID.String()),
		slog.String("relation_kind", payload.RelationKind),
		slog.String("branch_key", payload.BranchKey))
	return nil
}

// buildClusterBranch turns a Cluster candidate into a fully-populated branch:
// a new item that situates into a topic the user already follows. The
// four-tuple is always set, and the why is anchored (D28(a)): it names the
// anchor item's title in quotes, never a generic "a topic you follow" claim
// with no concrete reference back to what the user just read.
func buildClusterBranch(userID uuid.UUID, anchorItemKey, anchorTitle string, c sovereign_db.TrailClusterCandidate) BranchProposedPayload {
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
		AnchorItemKey:  anchorItemKey,
		RelationKind:   "cluster",
		Why:            fmt.Sprintf("Because you read %q — joins %s", anchorTitle, strings.Join(c.SharedTags, ", ")),
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
func buildContinuationBranch(userID uuid.UUID, c sovereign_db.TrailContinuationCandidate) BranchProposedPayload {
	return BranchProposedPayload{
		BranchKey:     "continuation:" + userID.String() + ":" + c.TargetItemKey,
		AnchorItemKey: c.TargetItemKey,
		RelationKind:  "continuation",
		Why:           fmt.Sprintf("Because you read %q and the thread went quiet — pick it back up.", c.TargetTitle),
		EvidenceRefs: []EvidenceRef{
			{RefID: c.TargetItemKey, Label: c.TargetTitle, Kind: "article"},
		},
		Confidence:     "plausible",
		TargetItemKey:  c.TargetItemKey,
		TargetTitle:    c.TargetTitle,
		PlannerVersion: plannerVersion,
	}
}
