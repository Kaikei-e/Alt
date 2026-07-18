package sovereign_db

import (
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
