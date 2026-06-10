// Package knowledge_trail_projector folds the append-only event log into the
// Knowledge Trail spine (knowledge_trail_footprints). It is reproject-safe:
// each footprint is derived from a single event's payload-resident fields, never
// from latest state or other read models. Re-running over the same log
// reproduces the same spine.
package knowledge_trail_projector

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/trail_planner"
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
	UpsertTrailBranch(ctx context.Context, userID, tenantID uuid.UUID, b sovereign_db.TrailBranch, createdAt time.Time, projectionVersion int) error
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
			if evt.EventType == trail_planner.EventTrailBranchProposed {
				if err := p.foldBranch(ctx, evt); err != nil {
					return err
				}
				continue
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

// foldBranch folds a trail.branch_proposed.v1 event into the branch read model.
// A branch missing the four-tuple is rejected loudly and never surfaced (Rule 4
// — untyped branches are the Loop decorated-feed failure). Malformed payloads
// are skipped without failing the batch (the event stays in the log).
func (p *Projector) foldBranch(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	if evt.UserID == nil {
		return nil
	}
	var payload trail_planner.BranchProposedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		p.logger.WarnContext(ctx, "trail projector: unparseable branch_proposed payload",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	if !payload.Valid() {
		p.logger.WarnContext(ctx, "trail projector: rejecting untyped branch_proposed",
			slog.String("branch_key", payload.BranchKey))
		return nil
	}
	refs := make([]sovereign_db.TrailEvidenceRef, len(payload.EvidenceRefs))
	for i, r := range payload.EvidenceRefs {
		refs[i] = sovereign_db.TrailEvidenceRef{RefID: r.RefID, Label: r.Label, Kind: r.Kind}
	}
	b := sovereign_db.TrailBranch{
		BranchKey:     payload.BranchKey,
		AnchorItemKey: payload.AnchorItemKey,
		RelationKind:  payload.RelationKind,
		Why:           payload.Why,
		EvidenceRefs:  refs,
		Confidence:    payload.Confidence,
		TargetItemKey: payload.TargetItemKey,
		TargetTitle:   payload.TargetTitle,
	}
	return p.repo.UpsertTrailBranch(ctx, *evt.UserID, evt.TenantID, b, evt.OccurredAt, 1)
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
