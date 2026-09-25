// Package knowledge_home_projector folds the Knowledge Sovereign append-only
// event log into the Knowledge Home read models (knowledge_home_items,
// today_digest_view, recall_candidate_view). It is reproject-safe: every
// business fact is derived from a single event's OccurredAt and
// payload-resident fields, never from alt-db lookups or other read models.
// Re-running over the same log reproduces the same read models.
//
// Ported from alt-backend/app/job/knowledge_projector.go per F-01
// (docs/review/architecturereview20260713.md): the fold logic used to live
// in alt-backend and reach knowledge-sovereign over an ApplyProjectionMutation
// RPC round trip. Moving it in-process (mirroring knowledge_trail_projector)
// removes that RPC hop. SummaryVersionCreated and TagSetVersionCreated no
// longer read the summary/tag body back from alt-db (GetSummaryVersionByID /
// GetTagSetVersionByID) — the producer now carries summary_text / tags on
// the event payload itself, and the fold uses them as-is.
package knowledge_home_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/projection_gap"
)

// Repository is the narrow surface the projector needs. Every write method
// takes the same json.RawMessage envelope sovereign_db.Repository already
// exposes for ApplyProjectionMutation, so *sovereign_db.Repository satisfies
// this interface directly — no driver changes, no intermediate gateway,
// same direct-wiring shape as knowledge_trail_projector.Repository.
type Repository interface {
	// The checkpoint is read as a token and advanced with a compare-and-set
	// against it. The seq-only pair (GetProjectionCheckpoint /
	// UpdateProjectionCheckpoint) is deliberately not in this surface: its
	// write is unconditional, so a batch that read the checkpoint before a
	// concurrent RebuildProjection would restore the pre-rebuild sequence over
	// the read models the rebuild had just emptied, and every event below it
	// would never be folded again (PM-2026-010).
	ReadProjectionCheckpointForAdvance(ctx context.Context, projectorName string) (sovereign_db.ProjectionCheckpoint, error)
	AdvanceProjectionCheckpointIfUnchanged(ctx context.Context, projectorName string, from sovereign_db.ProjectionCheckpoint, toSeq int64) (bool, error)
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error)
	// The gap frontier tells a hole in event_seq that a still-running writer
	// will fill apart from one a rollback burned. It re-reads the hole itself
	// rather than trusting the batch above, because the two reads land on
	// different snapshots — see usecase/projection_gap.
	ReadSequenceGapFrontier(ctx context.Context, firstSeq, lastSeq int64) (sovereign_db.SequenceGapFrontier, error)
	// GetActiveProjectionVersion resolves knowledge_projection_versions'
	// status='active' row — the same source of truth read paths already use
	// via activeProjectionVersionSQL (see driver/sovereign_db/sql_fragments.go),
	// but without that fragment's COALESCE(...,1) fallback: a missing active
	// version must fail the batch loudly, not silently regress to v1.
	GetActiveProjectionVersion(ctx context.Context) (*sovereign_db.ProjectionVersion, error)
	UpsertKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error
	DismissKnowledgeHomeItem(ctx context.Context, payload json.RawMessage) error
	ClearSupersedeState(ctx context.Context, payload json.RawMessage) error
	UpsertTodayDigest(ctx context.Context, payload json.RawMessage) error
	UpsertRecallCandidate(ctx context.Context, payload json.RawMessage) error
	PatchKnowledgeHomeItemURL(ctx context.Context, payload json.RawMessage) error
	SnoozeRecallCandidate(ctx context.Context, payload json.RawMessage) error
	DismissRecallCandidate(ctx context.Context, payload json.RawMessage) error
}

var _ Repository = (*sovereign_db.Repository)(nil)

// Config tunes batch sizing.
type Config struct {
	BatchSize         int
	MaxBatchesPerTick int
}

// Projector folds events into the Knowledge Home read models.
type Projector struct {
	repo   Repository
	logger *slog.Logger
	cfg    Config
	gaps   projection_gap.Tracker
}

// NewProjector builds a Projector. logger defaults to slog.Default() when
// nil; BatchSize/MaxBatchesPerTick default when <= 0.
func NewProjector(repo Repository, logger *slog.Logger, cfg Config) *Projector {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBatchSize
	}
	if cfg.MaxBatchesPerTick <= 0 {
		cfg.MaxBatchesPerTick = defaultMaxTick
	}
	return &Projector{repo: repo, logger: logger, cfg: cfg}
}

// RunBatch drains up to MaxBatchesPerTick batches from the event log, folding
// each of the 11 Knowledge Home event types (ArticleCreated,
// ArticleUrlBackfilled, SummaryVersionCreated, TagSetVersionCreated,
// HomeItemOpened, HomeItemDismissed, SummarySuperseded, TagSetSuperseded,
// ReasonMerged, RecallSnoozed, RecallDismissed) into the read models and
// advancing the checkpoint.
//
// A malformed/unparseable payload stops the batch without advancing the
// checkpoint past the failing event (it stays in the log to be retried),
// mirroring alt-backend/app/job/knowledge_projector.go's hard-fail-and-stop
// semantics. Unknown event types are skipped but still advance the
// checkpoint.
//
// The checkpoint is advanced with a compare-and-set against the state read at
// the start of the batch. If another writer moved it in the meantime the
// advance is refused, this tick's work is abandoned, and the next tick starts
// again from whatever that writer left behind.
func (p *Projector) RunBatch(ctx context.Context) error {
	for i := 0; i < p.cfg.MaxBatchesPerTick; i++ {
		checkpoint, err := p.repo.ReadProjectionCheckpointForAdvance(ctx, projectorName)
		if err != nil {
			return fmt.Errorf("read checkpoint: %w", err)
		}
		events, err := p.repo.ListKnowledgeEventsSince(ctx, checkpoint.LastEventSeq, p.cfg.BatchSize)
		if err != nil {
			return fmt.Errorf("list events: %w", err)
		}
		if len(events) == 0 {
			return nil
		}

		batch, hole := projection_gap.ContiguousPrefix(events, checkpoint.LastEventSeq)
		if len(batch) == 0 {
			abandon, err := p.mayAbandonHole(ctx, hole)
			if err != nil {
				return err
			}
			if !abandon {
				return nil
			}
			applied, err := p.repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, checkpoint, hole.Last)
			if err != nil {
				return fmt.Errorf("advance checkpoint: %w", err)
			}
			if !applied {
				p.logger.ErrorContext(ctx, "knowledge_home_projector.checkpoint_advance_rejected",
					slog.String("projector", projectorName),
					slog.Int64("expected_seq", checkpoint.LastEventSeq),
					slog.Int64("attempted_seq", hole.Last),
					slog.Int("batch_events", 0))
				return nil
			}
			continue
		}

		// Resolved once per batch — not per event (wasteful), not only at
		// process start (a reproject cutover would go unnoticed until
		// restart). This is projection metadata, the same class the read
		// path already resolves via activeProjectionVersionSQL, so reading
		// it here does not compromise reproject-safety: every business
		// fact folded below still comes from the event payload alone.
		activeVersion, err := p.repo.GetActiveProjectionVersion(ctx)
		if err != nil {
			return fmt.Errorf("get active projection version: %w", err)
		}
		if activeVersion == nil {
			return fmt.Errorf("knowledge_home_projector: no active projection version configured")
		}

		lastGoodSeq := checkpoint.LastEventSeq
		var foldErr error
		for _, evt := range batch {
			if err := p.foldEvent(ctx, evt, activeVersion.Version); err != nil {
				foldErr = fmt.Errorf("fold event %s (seq=%d): %w", evt.EventType, evt.EventSeq, err)
				break
			}
			lastGoodSeq = evt.EventSeq
		}

		// Advance the checkpoint up to (and including) the last
		// successfully-folded event even when the batch stopped early on a
		// hard failure — the failing event itself is never skipped past.
		if lastGoodSeq > checkpoint.LastEventSeq {
			applied, err := p.repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, checkpoint, lastGoodSeq)
			if err != nil {
				return fmt.Errorf("advance checkpoint: %w", err)
			}
			if !applied {
				// Somebody else wrote the checkpoint while this batch was
				// folding: an operator's RebuildProjection, or another
				// service's reproject swap. Their value is the authoritative
				// one and this batch is working from a stale view of where the
				// projection stands, so the tick ends here and the next one
				// re-reads. Retrying with a fresh token would re-apply exactly
				// the write the other writer was trying to prevent, and the
				// rebuilt read models would be left empty behind a checkpoint
				// at the tip (PM-2026-010). Nothing is lost by stopping: the
				// events are still in the append-only log and the folds are
				// idempotent, so the next tick re-folds them.
				p.logger.ErrorContext(ctx, "knowledge_home_projector.checkpoint_advance_rejected",
					slog.String("projector", projectorName),
					slog.Int64("expected_seq", checkpoint.LastEventSeq),
					slog.Int64("attempted_seq", lastGoodSeq),
					slog.Int("batch_events", len(batch)))
				return foldErr
			}
		}
		if foldErr != nil {
			return foldErr
		}
		if hole.Open() {
			return nil
		}
		if len(events) < p.cfg.BatchSize {
			return nil
		}
	}
	return nil
}

// mayAbandonHole reports whether the sequences blocking this batch may be
// treated as burned by rolled-back transactions rather than as ones writers
// still in flight are about to commit.
func (p *Projector) mayAbandonHole(ctx context.Context, hole projection_gap.Hole) (bool, error) {
	frontier, err := p.repo.ReadSequenceGapFrontier(ctx, hole.First, hole.Last)
	if err != nil {
		return false, fmt.Errorf("read sequence gap frontier: %w", err)
	}
	if !p.gaps.MayAbandon(hole, frontier) {
		p.logger.InfoContext(ctx, "knowledge_home_projector.sequence_gap_waiting",
			slog.String("projector", projectorName),
			slog.Int64("gap_seq", hole.First),
			slog.Int64("gap_through", hole.Last),
			slog.Int64("xmin", frontier.Xmin),
			slog.Int64("ceiling_xid", frontier.Ceiling),
			slog.Bool("hole_open", frontier.HoleOpen))
		return false, nil
	}
	p.logger.WarnContext(ctx, "knowledge_home_projector.sequence_gap_abandoned",
		slog.String("projector", projectorName),
		slog.Int64("gap_seq", hole.First),
		slog.Int64("gap_through", hole.Last))
	return true, nil
}

// foldEvent dispatches a single event to its fold function, stamping every
// write with version. RunBatch passes the batch's resolved active version;
// a reproject/backfill caller may instead pass an explicit target version
// (e.g. building a shadow version while the active one stays live) — this
// function has no opinion on where version comes from.
func (p *Projector) foldEvent(ctx context.Context, evt sovereign_db.KnowledgeEvent, version int) error {
	switch evt.EventType {
	case "ArticleCreated":
		return p.foldArticleCreated(ctx, evt, version)
	case "ArticleUrlBackfilled":
		return p.foldArticleUrlBackfilled(ctx, evt, version)
	case "SummaryVersionCreated":
		return p.foldSummaryVersionCreated(ctx, evt, version)
	case "TagSetVersionCreated":
		return p.foldTagSetVersionCreated(ctx, evt, version)
	case "HomeItemOpened":
		return p.foldHomeItemOpened(ctx, evt, version)
	case "HomeItemDismissed":
		return p.foldHomeItemDismissed(ctx, evt, version)
	case "SummarySuperseded":
		return p.foldSummarySuperseded(ctx, evt, version)
	case "TagSetSuperseded":
		return p.foldTagSetSuperseded(ctx, evt, version)
	case "ReasonMerged":
		return p.foldReasonMerged(ctx, evt, version)
	case "RecallSnoozed":
		return p.foldRecallSnoozed(ctx, evt)
	case "RecallDismissed":
		return p.foldRecallDismissed(ctx, evt)
	default:
		// Unknown event types are silently skipped but still advance the
		// checkpoint (handled by the caller).
		return nil
	}
}

// ── repository call helpers ──

func (p *Projector) upsertHomeItem(ctx context.Context, item homeItemWrite) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal knowledge_home_items payload: %w", err)
	}
	if err := p.repo.UpsertKnowledgeHomeItem(ctx, raw); err != nil {
		return fmt.Errorf("upsert knowledge_home_items: %w", err)
	}
	return nil
}

// upsertDigest's error is fatal to the fold, unlike the recall-candidate and
// clear-supersede side effects below. today_digest_view's counters are
// additive deltas keyed on the folding event's event_seq: an event the
// checkpoint walks past contributes its increment exactly never again, and a
// RebuildProjection cannot recover it either — the rebuild truncates and
// replays the same log into the same failure. Logging at WARN and returning
// nil here is also precisely what hid the producer/consumer skew that stopped
// today_digest_view being written at all (Alt Rule 8).
func (p *Projector) upsertDigest(ctx context.Context, digest digestWrite) error {
	raw, err := json.Marshal(digest)
	if err != nil {
		return fmt.Errorf("marshal today_digest_view payload: %w", err)
	}
	if err := p.repo.UpsertTodayDigest(ctx, raw); err != nil {
		return fmt.Errorf("upsert today_digest_view: %w", err)
	}
	return nil
}

func (p *Projector) upsertRecallCandidate(ctx context.Context, candidate recallCandidateWrite) error {
	raw, err := json.Marshal(candidate)
	if err != nil {
		return fmt.Errorf("marshal recall_candidate_view payload: %w", err)
	}
	return p.repo.UpsertRecallCandidate(ctx, raw)
}

func (p *Projector) clearSupersedeState(ctx context.Context, params clearSupersedeWrite) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal clear_supersede_state payload: %w", err)
	}
	return p.repo.ClearSupersedeState(ctx, raw)
}
