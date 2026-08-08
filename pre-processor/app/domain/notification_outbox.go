package domain

import "time"

// NotificationKind names the notification_outbox kinds pre-processor produces.
type NotificationKind string

// NotificationKindSummaryReady is emitted when a summarize job commits its
// summary, and is the discriminator both the outbox row and its payload carry.
const NotificationKindSummaryReady NotificationKind = "summary_ready"

// NotificationOutboxRow is one claimed notification_outbox row, carrying only
// the fields the relay needs to forward it to alt-data-hub.
type NotificationOutboxRow struct {
	ID         string
	DedupeKey  string
	UserID     string
	Kind       string
	Payload    []byte
	OccurredAt time.Time
	// Attempts is the count after the claim incremented it, so the first
	// forward of a fresh row sees 1.
	Attempts int
}
