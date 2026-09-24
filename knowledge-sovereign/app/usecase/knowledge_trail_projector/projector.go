// Package knowledge_trail_projector folds the append-only event log into the
// Knowledge Trail spine (knowledge_trail_footprints). It is reproject-safe:
// each footprint is derived from a single event's payload-resident fields, never
// from latest state or other read models. Re-running over the same log
// reproduces the same spine.
package knowledge_trail_projector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/projection_gap"
	"knowledge-sovereign/usecase/trail_planner"
)

const (
	projectorName    = "knowledge-trail-projector"
	defaultBatchSize = 500
	defaultMaxTick   = 4

	// trailProjectionVersion stamps every projected row. Bumped to 2 when the
	// act-outcome side table joined the projection (D22; docs/runbooks/knowledge-trail-reproject.md).
	trailProjectionVersion = 2

	// eventTrailActOutcome is the current dwell-outcome vocabulary (D16).
	eventTrailActOutcome = "trail.act_outcome.v1"
	// eventLegacyActOutcome is the Loop-era vocabulary: history-only, never
	// emitted anew, but its rows keep feeding path wear verbatim (D18/D20).
	eventLegacyActOutcome = "knowledge_loop.act_outcome.v1"
)

// verbByEventType maps the canonical user-action event types to the user-facing
// footprint verb. Only events present here become footprints; everything else
// advances the checkpoint without emitting a footprint.
var verbByEventType = map[string]string{
	"HomeItemOpened":          "read",
	"HomeItemAsked":           "asked",
	"HomeItemListened":        "listened",
	"HomeItemDismissed":       "dismissed",
	"knowledge_loop.acted.v1": "read", // historical loop engagement projects as a read footprint
}

// Repository is the narrow surface the projector needs.
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
	UpsertTrailFootprint(ctx context.Context, fp sovereign_db.TrailFootprint, projectionVersion int) error
	UpsertTrailBranch(ctx context.Context, userID, tenantID uuid.UUID, b sovereign_db.TrailBranch, createdAt time.Time, projectionVersion int) error
	SetTrailBranchState(ctx context.Context, userID uuid.UUID, branchKey, state string) error
	InsertTrailActOutcome(ctx context.Context, o sovereign_db.TrailActOutcome, projectionVersion int) error
}

// Config tunes batch sizing.
type Config struct {
	BatchSize         int
	MaxBatchesPerTick int
}

// Projector folds events into the trail spine.
type Projector struct {
	repo   Repository
	logger *slog.Logger
	cfg    Config
	gaps   projection_gap.Tracker
}

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

// RunBatch processes the next batch of knowledge events, updates footprints,
// and advances the checkpoint. It returns nil when the event stream is caught up.
//
// knowledge_events.event_seq is taken at INSERT, not COMMIT. Gaps in the sequence
// space stop the batch at the last contiguous event so out-of-order commits cannot
// leave holes behind an advanced checkpoint. When a hole persists past the
// abandonment window, the projector skips it so an abandoned sequence number
// cannot stall projection indefinitely.
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
			return err
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
				p.logger.ErrorContext(ctx, "trail.checkpoint_advance_rejected",
					slog.String("projector", projectorName),
					slog.Int64("expected_seq", checkpoint.LastEventSeq),
					slog.Int64("attempted_seq", hole.Last),
					slog.Int("batch_events", 0))
				return nil
			}
			continue
		}

		var maxSeq int64
		for _, evt := range batch {
			if evt.EventSeq > maxSeq {
				maxSeq = evt.EventSeq
			}
			if evt.EventType == trail_planner.EventTrailBranchProposed {
				if err := p.foldBranch(ctx, evt); err != nil {
					return err
				}
				continue
			}
			if evt.EventType == trail_planner.EventTrailBranchResolved {
				if err := p.foldBranchResolved(ctx, evt); err != nil {
					return err
				}
				continue
			}
			if evt.EventType == eventTrailActOutcome || evt.EventType == eventLegacyActOutcome {
				if err := p.foldActOutcome(ctx, evt); err != nil {
					return err
				}
				continue
			}
			fp, ok := footprintFromEvent(evt)
			if !ok {
				continue
			}
			if err := p.repo.UpsertTrailFootprint(ctx, fp, trailProjectionVersion); err != nil {
				return err
			}
		}

		applied, err := p.repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, checkpoint, maxSeq)
		if err != nil {
			return fmt.Errorf("advance checkpoint: %w", err)
		}
		if !applied {
			// Somebody else wrote the checkpoint while this batch was folding:
			// an operator's RebuildProjection, or another service's reproject
			// swap. Their value is the authoritative one and this batch is
			// working from a stale view of where the projection stands, so the
			// tick ends here and the next one re-reads. Retrying with a fresh
			// token would re-apply exactly the write the other writer was
			// trying to prevent, and the rebuilt spine would be left empty
			// behind a checkpoint at the tip (PM-2026-010). Nothing is lost by
			// stopping: the events are still in the append-only log and the
			// folds are idempotent, so the next tick re-folds them.
			p.logger.ErrorContext(ctx, "trail.checkpoint_advance_rejected",
				slog.String("projector", projectorName),
				slog.Int64("expected_seq", checkpoint.LastEventSeq),
				slog.Int64("attempted_seq", maxSeq),
				slog.Int("batch_events", len(batch)))
			return nil
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
		p.logger.InfoContext(ctx, "trail.sequence_gap_waiting",
			slog.String("projector", projectorName),
			slog.Int64("gap_seq", hole.First),
			slog.Int64("gap_through", hole.Last),
			slog.Int64("xmin", frontier.Xmin),
			slog.Int64("ceiling_xid", frontier.Ceiling),
			slog.Bool("hole_open", frontier.HoleOpen))
		return false, nil
	}
	p.logger.WarnContext(ctx, "trail.sequence_gap_abandoned",
		slog.String("projector", projectorName),
		slog.Int64("gap_seq", hole.First),
		slog.Int64("gap_through", hole.Last))
	return true, nil
}
