package reading_projector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/projection_gap"
)

const (
	projectorName    = "reading-projector"
	defaultBatchSize = 500
	defaultMaxTick   = 40
)

// Repository is the narrow persistence surface required by reading_projector.
type Repository interface {
	ReadProjectionCheckpointForAdvance(ctx context.Context, projectorName string) (sovereign_db.ProjectionCheckpoint, error)
	AdvanceProjectionCheckpointIfUnchanged(ctx context.Context, projectorName string, from sovereign_db.ProjectionCheckpoint, toSeq int64) (bool, error)
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error)
	ReadSequenceGapFrontier(ctx context.Context, firstSeq, lastSeq int64) (sovereign_db.SequenceGapFrontier, error)
	UpsertReadingSource(ctx context.Context, src sovereign_db.ReadingSource) error
	UpsertReadingQueueItem(ctx context.Context, item sovereign_db.ReadingQueueItem) error
	RemoveReadingQueueItem(ctx context.Context, userID, articleID uuid.UUID, removedAt time.Time, reason string, eventSeq int64) error
}

var _ Repository = (*sovereign_db.Repository)(nil)

// Config tunes batch sizing.
type Config struct {
	BatchSize         int
	MaxBatchesPerTick int
}

// Projector folds reading events into reading_sources and reading_queue_items.
type Projector struct {
	repo   Repository
	logger *slog.Logger
	cfg    Config
	gaps   projection_gap.Tracker
	behind bool
}

// NewProjector builds a reading Projector.
func NewProjector(repo Repository, logger *slog.Logger, cfg Config) *Projector {
	if repo == nil {
		panic("reading_projector: repository is required")
	}
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

// RunBatch processes up to MaxBatchesPerTick contiguous event batches and
// advances the reading-projector checkpoint with CAS protection.
func (p *Projector) RunBatch(ctx context.Context) error {
	var lastAdvancedSeq int64
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
			p.markCaughtUp(ctx, checkpoint.LastEventSeq)
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
				p.logger.ErrorContext(ctx, "reading_projector.checkpoint_advance_rejected",
					slog.String("projector", projectorName),
					slog.Int64("expected_seq", checkpoint.LastEventSeq),
					slog.Int64("attempted_seq", hole.Last),
					slog.Int("batch_events", 0))
				return nil
			}
			lastAdvancedSeq = hole.Last
			if len(events) < p.cfg.BatchSize {
				p.markCaughtUp(ctx, hole.Last)
				return nil
			}
			continue
		}

		lastGoodSeq := checkpoint.LastEventSeq
		var foldErr error
		for _, evt := range batch {
			if err := p.foldEvent(ctx, evt); err != nil {
				foldErr = fmt.Errorf("fold event %s (seq=%d): %w", evt.EventType, evt.EventSeq, err)
				break
			}
			lastGoodSeq = evt.EventSeq
		}

		if lastGoodSeq > checkpoint.LastEventSeq {
			applied, err := p.repo.AdvanceProjectionCheckpointIfUnchanged(ctx, projectorName, checkpoint, lastGoodSeq)
			if err != nil {
				return fmt.Errorf("advance checkpoint: %w", err)
			}
			if !applied {
				p.logger.ErrorContext(ctx, "reading_projector.checkpoint_advance_rejected",
					slog.String("projector", projectorName),
					slog.Int64("expected_seq", checkpoint.LastEventSeq),
					slog.Int64("attempted_seq", lastGoodSeq),
					slog.Int("batch_events", len(batch)))
				return foldErr
			}
			lastAdvancedSeq = lastGoodSeq
		}
		if foldErr != nil {
			return foldErr
		}
		if hole.Open() {
			return nil
		}
		if len(events) < p.cfg.BatchSize {
			p.markCaughtUp(ctx, lastGoodSeq)
			return nil
		}
	}
	p.markBehind(ctx, lastAdvancedSeq)
	return nil
}

func (p *Projector) markBehind(ctx context.Context, lastEventSeq int64) {
	if p.behind {
		return
	}
	p.behind = true
	p.logger.InfoContext(ctx, "reading_projector.catchup_started",
		slog.String("projector", projectorName),
		slog.Int64("last_event_seq", lastEventSeq),
		slog.Int("batch_size", p.cfg.BatchSize),
		slog.Int("max_batches_per_tick", p.cfg.MaxBatchesPerTick))
}

func (p *Projector) markCaughtUp(ctx context.Context, lastEventSeq int64) {
	if !p.behind {
		return
	}
	p.behind = false
	p.logger.InfoContext(ctx, "reading_projector.catchup_completed",
		slog.String("projector", projectorName),
		slog.Int64("last_event_seq", lastEventSeq))
}

func (p *Projector) mayAbandonHole(ctx context.Context, hole projection_gap.Hole) (bool, error) {
	frontier, err := p.repo.ReadSequenceGapFrontier(ctx, hole.First, hole.Last)
	if err != nil {
		return false, fmt.Errorf("read sequence gap frontier: %w", err)
	}
	if !p.gaps.MayAbandon(hole, frontier) {
		p.logger.InfoContext(ctx, "reading_projector.sequence_gap_waiting",
			slog.String("projector", projectorName),
			slog.Int64("gap_seq", hole.First),
			slog.Int64("gap_through", hole.Last),
			slog.Int64("xmin", frontier.Xmin),
			slog.Int64("ceiling_xid", frontier.Ceiling),
			slog.Bool("hole_open", frontier.HoleOpen))
		return false, nil
	}
	p.logger.WarnContext(ctx, "reading_projector.sequence_gap_abandoned",
		slog.String("projector", projectorName),
		slog.Int64("gap_seq", hole.First),
		slog.Int64("gap_through", hole.Last))
	return true, nil
}
