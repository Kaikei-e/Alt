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
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
)

// Repository is the narrow surface the planner needs.
type Repository interface {
	ListDistinctUserIDs(ctx context.Context) ([]uuid.UUID, error)
	GetLatestFootprintAnchor(ctx context.Context, userID uuid.UUID) (sovereign_db.FootprintAnchor, bool, error)
	GetItemTitle(ctx context.Context, userID uuid.UUID, itemKey string) (string, bool, error)
	DeriveTrailClusterCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]sovereign_db.TrailClusterCandidate, error)
	DeriveTrailContinuationCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]sovereign_db.TrailContinuationCandidate, error)
	// AppendKnowledgeEventIfNew appends an event only if its dedupe_key has
	// not been seen yet, returning (event_seq, true, nil) when fresh and (0,
	// false, nil) on duplicate. The bool return tells the caller whether it
	// appended. The planner may only claim a proposal the log accepted, so
	// the seq-only form (whose 0 means either "rejected" or "you did not
	// look") is deliberately not in this surface.
	AppendKnowledgeEventIfNew(ctx context.Context, event sovereign_db.KnowledgeEvent) (int64, bool, error)
}

// Config tunes the planner.
type Config struct {
	MaxBranchesPerUser int
	// Clock is injected so the emitted occurred_at is testable and the planner
	// holds no wall clock literal. Production wires wall clock.
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
	// never silently no-op.
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
	anchor, ok, err := p.repo.GetLatestFootprintAnchor(ctx, userID)
	if err != nil {
		return fmt.Errorf("trail_planner anchor: %w", err)
	}
	if !ok {
		// Either no footprints yet, or none the why can name (a spine of
		// dismissals). Both mean the same thing here: nothing to fork from.
		p.logger.WarnContext(ctx, "trail.branch_anchor_unresolved",
			slog.String("user_id", userID.String()),
			slog.String("reason", "no_eligible_footprint"))
		return nil
	}

	// §C4: the why names the act that happened. A verb with no phrase is a
	// verb the why cannot back, so it suppresses rather than borrowing "read".
	whyPhrase, phraseOK := whyPhraseForVerb(anchor.Verb)
	if !phraseOK {
		p.logger.WarnContext(ctx, "trail.branch_anchor_unresolved",
			slog.String("user_id", userID.String()),
			slog.String("anchor_item_key", anchor.ItemKey),
			slog.String("reason", "unphrasable_verb"),
			slog.String("verb", anchor.Verb))
		return nil
	}

	// D28(a): a branch whose why does not reference its anchor is forbidden.
	// The anchor's title is that reference, so an unresolvable title must
	// suppress emission for this user — never fall back to a generic why.
	anchorTitle, titleOK, err := p.repo.GetItemTitle(ctx, userID, anchor.ItemKey)
	if err != nil {
		return fmt.Errorf("trail_planner anchor title: %w", err)
	}
	if !titleOK {
		p.logger.WarnContext(ctx, "trail.branch_anchor_unresolved",
			slog.String("user_id", userID.String()),
			slog.String("anchor_item_key", anchor.ItemKey),
			slog.String("reason", "title_unresolved"))
		return nil
	}
	ref := anchorRef{itemKey: anchor.ItemKey, title: anchorTitle, whyPhrase: whyPhrase}

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
		payload := buildClusterBranch(userID, ref, c)
		if err := p.emitBranch(ctx, userID, anchor.TenantID, payload); err != nil {
			return err
		}
	}

	if err := p.planContinuationBranch(ctx, userID, anchor.TenantID); err != nil {
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
			slog.String("anchor_item_key", c.TargetItemKey),
			slog.String("reason", "title_unresolved"))
		return nil
	}
	// Self-referential anchor: the why is phrased from the contact that
	// qualified this thread, not from the user's latest footprint.
	whyPhrase, ok := whyPhraseForVerb(c.Verb)
	if !ok {
		p.logger.WarnContext(ctx, "trail.branch_anchor_unresolved",
			slog.String("user_id", userID.String()),
			slog.String("anchor_item_key", c.TargetItemKey),
			slog.String("reason", "unphrasable_verb"),
			slog.String("verb", c.Verb))
		return nil
	}
	payload := buildContinuationBranch(userID, whyPhrase, c)
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
	evt, err := buildBranchProposedEvent(userID, tenantID, payload, p.cfg.Clock())
	if err != nil {
		return err
	}
	seq, appended, err := p.repo.AppendKnowledgeEventIfNew(ctx, evt)
	if err != nil {
		return fmt.Errorf("trail_planner emit: %w", err)
	}
	if !appended {
		// The branch was already proposed once; re-proposal is correctly
		// rejected by the dedupe registry. Record it as a rejection — logging
		// it as a proposal would claim work the event log never accepted.
		p.logger.InfoContext(ctx, "trail.branch_dedupe_rejected",
			slog.String("user_id", userID.String()),
			slog.String("relation_kind", payload.RelationKind),
			slog.String("branch_key", payload.BranchKey))
		return nil
	}
	p.logger.InfoContext(ctx, "trail.branch_proposed",
		slog.String("user_id", userID.String()),
		slog.String("relation_kind", payload.RelationKind),
		slog.String("branch_key", payload.BranchKey),
		slog.Int64("event_seq", seq))
	return nil
}
