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
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &Planner{repo: repo, logger: logger, cfg: cfg}
}

// RunBatch derives branches for every user with eligible trail history and
// emits a branch_proposed event per fresh candidate (idempotent via dedupe_key).
func (p *Planner) RunBatch(ctx context.Context) error {
	// A planner reached with no repository is a wiring bug — fail loud, never
	// silently no-op.
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
	return p.planContinuationBranch(ctx, userID)
}

// planContinuationBranch derives at most one Continuation candidate per user
// per run and emits it. Continuation is self-referential: the target IS the
// anchor, because past engagement with the same item qualifies it.
func (p *Planner) planContinuationBranch(ctx context.Context, userID uuid.UUID) error {
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
	return p.emitBranch(ctx, userID, c.TenantID, payload)
}

// emitBranch appends a validated branch_proposed event and logs its relation
// kind. A branch that fails Valid() is dropped rather than emitted incomplete.
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
