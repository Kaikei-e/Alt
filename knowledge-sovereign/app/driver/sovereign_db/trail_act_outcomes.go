package sovereign_db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TrailActOutcome is one observed consequence of a taken trail branch,
// projected insert-only from the event log (D20: outcomes never add rows to
// the spine — they feed path wear as a side table).
//
// Exactly one of DwellMs / LegacyOutcome is set: trail.act_outcome.v1 events
// carry the raw dwell measurement; historical knowledge_loop.act_outcome.v1
// events carry their era's classified label verbatim (never faked into ms).
type TrailActOutcome struct {
	UserID          uuid.UUID
	TenantID        uuid.UUID
	OutcomeKey      string
	BranchKey       string // empty for legacy loop outcomes
	ItemKey         string
	DwellMs         *int64 // nil for legacy outcomes
	LegacyOutcome   string // empty for trail outcomes
	SourceEventType string
	OccurredAt      time.Time
}

// InsertTrailActOutcome records one outcome insert-only. First write wins per
// outcome_key (the event dedupe key), so replays and retries are no-ops.
func (r *Repository) InsertTrailActOutcome(ctx context.Context, o TrailActOutcome, projectionVersion int) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO knowledge_trail_act_outcomes
  (user_id, tenant_id, outcome_key, branch_key, item_key, dwell_ms,
   legacy_outcome, source_event_type, occurred_at, projection_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (user_id, outcome_key) DO NOTHING`,
		o.UserID, o.TenantID, o.OutcomeKey, o.BranchKey, o.ItemKey, o.DwellMs,
		o.LegacyOutcome, o.SourceEventType, o.OccurredAt, projectionVersion)
	if err != nil {
		return fmt.Errorf("InsertTrailActOutcome: %w", err)
	}
	return nil
}
