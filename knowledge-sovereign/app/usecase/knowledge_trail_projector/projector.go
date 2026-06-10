// Package knowledge_trail_projector folds the append-only event log into the
// Knowledge Trail spine (knowledge_trail_footprints). It is reproject-safe:
// each footprint is derived from a single event's payload-resident fields, never
// from latest state or other read models. Re-running over the same log
// reproduces the same spine.
package knowledge_trail_projector

import (
	"context"
	"log/slog"

	"knowledge-sovereign/driver/sovereign_db"
)

const (
	projectorName    = "knowledge-trail-projector"
	defaultBatchSize = 500
	defaultMaxTick   = 4
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
	GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error)
	UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error)
	UpsertTrailFootprint(ctx context.Context, fp sovereign_db.TrailFootprint, projectionVersion int) error
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

// RunBatch drains up to MaxBatchesPerTick batches from the event log, folding
// each act event into a footprint and advancing the checkpoint.
func (p *Projector) RunBatch(ctx context.Context) error {
	for i := 0; i < p.cfg.MaxBatchesPerTick; i++ {
		lastSeq, err := p.repo.GetProjectionCheckpoint(ctx, projectorName)
		if err != nil {
			return err
		}
		events, err := p.repo.ListKnowledgeEventsSince(ctx, lastSeq, p.cfg.BatchSize)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}

		var maxSeq int64
		for _, evt := range events {
			if evt.EventSeq > maxSeq {
				maxSeq = evt.EventSeq
			}
			fp, ok := footprintFromEvent(evt)
			if !ok {
				continue
			}
			if err := p.repo.UpsertTrailFootprint(ctx, fp, 1); err != nil {
				return err
			}
		}

		if err := p.repo.UpdateProjectionCheckpoint(ctx, projectorName, maxSeq); err != nil {
			return err
		}
		if len(events) < p.cfg.BatchSize {
			return nil
		}
	}
	return nil
}

// footprintFromEvent derives a footprint from an act event. Returns ok=false for
// non-act events (which still advance the checkpoint) and for system events with
// no user_id.
func footprintFromEvent(evt sovereign_db.KnowledgeEvent) (sovereign_db.TrailFootprint, bool) {
	verb, ok := verbByEventType[evt.EventType]
	if !ok || evt.UserID == nil || evt.AggregateID == "" {
		return sovereign_db.TrailFootprint{}, false
	}
	key := evt.DedupeKey
	if key == "" {
		key = evt.EventID.String()
	}
	return sovereign_db.TrailFootprint{
		UserID:          *evt.UserID,
		TenantID:        evt.TenantID,
		FootprintKey:    key,
		Verb:            verb,
		ItemKey:         evt.AggregateID,
		SourceEventType: evt.EventType,
		OccurredAt:      evt.OccurredAt,
	}, true
}
