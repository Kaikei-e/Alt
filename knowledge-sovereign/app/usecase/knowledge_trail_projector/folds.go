package knowledge_trail_projector

import (
	"context"
	"encoding/json"
	"log/slog"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/trail_planner"
)

// buildBranchFromEvent derives a typed TrailBranch from a branch_proposed event.
// Returns (nil, payload, false, nil) for events with untyped four-tuple.
func buildBranchFromEvent(evt sovereign_db.KnowledgeEvent) (*sovereign_db.TrailBranch, trail_planner.BranchProposedPayload, bool, error) {
	var payload trail_planner.BranchProposedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return nil, payload, false, err
	}
	if !payload.Valid() {
		return nil, payload, false, nil
	}
	refs := make([]sovereign_db.TrailEvidenceRef, len(payload.EvidenceRefs))
	for i, r := range payload.EvidenceRefs {
		refs[i] = sovereign_db.TrailEvidenceRef{RefID: r.RefID, Label: r.Label, Kind: r.Kind}
	}
	return &sovereign_db.TrailBranch{
		BranchKey:     payload.BranchKey,
		AnchorItemKey: payload.AnchorItemKey,
		RelationKind:  payload.RelationKind,
		Why:           payload.Why,
		EvidenceRefs:  refs,
		Confidence:    payload.Confidence,
		TargetItemKey: payload.TargetItemKey,
		TargetTitle:   payload.TargetTitle,
	}, payload, true, nil
}

// buildBranchResolutionFromEvent extracts and validates the resolution from a branch_resolved event.
func buildBranchResolutionFromEvent(evt sovereign_db.KnowledgeEvent) (trail_planner.BranchResolvedPayload, bool, error) {
	if evt.UserID == nil {
		return trail_planner.BranchResolvedPayload{}, false, nil
	}
	var payload trail_planner.BranchResolvedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return trail_planner.BranchResolvedPayload{}, false, err
	}
	if payload.BranchKey == "" || !trail_planner.ValidResolution(payload.Resolution) {
		return payload, false, nil
	}
	return payload, true, nil
}

// buildActOutcomeFromEvent derives a TrailActOutcome from a dwell-outcome event.
// trail.act_outcome.v1 carries raw dwell; knowledge_loop.act_outcome.v1 keeps its classified label.
func buildActOutcomeFromEvent(evt sovereign_db.KnowledgeEvent) (*sovereign_db.TrailActOutcome, bool, error) {
	if evt.UserID == nil {
		return nil, false, nil
	}
	var payload struct {
		BranchKey string `json:"branch_key"`
		ItemKey   string `json:"item_key"`
		EntryKey  string `json:"entry_key"`
		DwellMs   *int64 `json:"dwell_ms"`
		Outcome   string `json:"outcome"`
	}
	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return nil, false, err
	}
	key := evt.DedupeKey
	if key == "" {
		key = evt.EventID.String()
	}
	o := sovereign_db.TrailActOutcome{
		UserID:          *evt.UserID,
		TenantID:        evt.TenantID,
		OutcomeKey:      key,
		SourceEventType: evt.EventType,
		OccurredAt:      evt.OccurredAt,
	}
	switch evt.EventType {
	case eventTrailActOutcome:
		if payload.BranchKey == "" || payload.ItemKey == "" || payload.DwellMs == nil || *payload.DwellMs < 0 {
			return nil, false, nil
		}
		o.BranchKey = payload.BranchKey
		o.ItemKey = payload.ItemKey
		o.DwellMs = payload.DwellMs
	default: // eventLegacyActOutcome
		itemKey := payload.EntryKey
		if itemKey == "" {
			itemKey = payload.ItemKey
		}
		if itemKey == "" {
			itemKey = evt.AggregateID
		}
		if itemKey == "" || payload.Outcome == "" {
			return nil, false, nil
		}
		o.ItemKey = itemKey
		o.LegacyOutcome = payload.Outcome
	}
	return &o, true, nil
}

// footprintFromEvent derives a footprint from an act event. Returns ok=false for
// non-act events and for system events with no user_id.
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

func (p *Projector) foldBranch(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	if evt.UserID == nil {
		p.logger.WarnContext(ctx, "trail projector: rejecting branch_proposed with no user_id",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	branch, payload, ok, err := buildBranchFromEvent(evt)
	if err != nil {
		p.logger.WarnContext(ctx, "trail projector: unparseable branch_proposed payload",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	if !ok {
		p.logger.WarnContext(ctx, "trail projector: rejecting untyped branch_proposed",
			slog.String("branch_key", payload.BranchKey))
		return nil
	}
	return p.repo.UpsertTrailBranch(ctx, *evt.UserID, evt.TenantID, *branch, evt.OccurredAt, trailProjectionVersion)
}

func (p *Projector) foldBranchResolved(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	if evt.UserID == nil {
		p.logger.WarnContext(ctx, "trail projector: rejecting branch_resolved with no user_id",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	payload, ok, err := buildBranchResolutionFromEvent(evt)
	if err != nil {
		p.logger.WarnContext(ctx, "trail projector: unparseable branch_resolved payload",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	if !ok {
		p.logger.WarnContext(ctx, "trail projector: rejecting invalid branch_resolved",
			slog.String("branch_key", payload.BranchKey),
			slog.String("resolution", payload.Resolution))
		return nil
	}
	if err := p.repo.SetTrailBranchState(ctx, *evt.UserID, payload.BranchKey, payload.Resolution); err != nil {
		return err
	}
	// Wave 10 branch KPI: resolution + whether a dismiss reason (D28(d)) was
	// supplied. The measured outcome is taken→engaged dwell, not CTR — see
	// foldActOutcome — but resolution/reason presence is the raw signal the
	// ClickHouse pipeline (rask) aggregates for it.
	p.logger.InfoContext(ctx, "trail.branch_resolved",
		slog.String("resolution", payload.Resolution),
		slog.Bool("has_reason", payload.DismissReason != ""))
	return nil
}

func (p *Projector) foldActOutcome(ctx context.Context, evt sovereign_db.KnowledgeEvent) error {
	if evt.UserID == nil {
		p.logger.WarnContext(ctx, "trail projector: rejecting act_outcome with no user_id",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	outcome, ok, err := buildActOutcomeFromEvent(evt)
	if err != nil {
		p.logger.WarnContext(ctx, "trail projector: unparseable act_outcome payload",
			slog.String("event_id", evt.EventID.String()))
		return nil
	}
	if !ok {
		msg := "trail projector: rejecting incomplete legacy act_outcome"
		if evt.EventType == eventTrailActOutcome {
			msg = "trail projector: rejecting incomplete trail act_outcome"
		}
		p.logger.WarnContext(ctx, msg, slog.String("event_id", evt.EventID.String()))
		return nil
	}
	if evt.EventType == eventTrailActOutcome {
		// Wave 10 branch KPI: raw dwell + whether it crosses the engaged
		// threshold (taken→engaged dwell, not CTR — D28(c)). Reuses the same
		// read-time constant the wear derivation uses, never a duplicated
		// literal.
		p.logger.InfoContext(ctx, "trail.act_outcome.observed",
			slog.Int64("dwell_ms", *outcome.DwellMs),
			slog.Bool("engaged", *outcome.DwellMs >= sovereign_db.EngagedDwellMs))
	}
	return p.repo.InsertTrailActOutcome(ctx, *outcome, trailProjectionVersion)
}
