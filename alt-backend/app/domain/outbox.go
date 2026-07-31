package domain

import "time"

// OutboxEventStatus is an outbox_events row's position in its state machine.
//
// It is a named type rather than a bare string because the driver it replaces
// took `status string` and let every caller spell the transition itself. Two
// distinct operations — releasing an unattempted claim and recording a
// terminal outcome — were the same call with a different literal, and the
// difference between them (only terminal statuses stamp processed_at) lived in
// an `if` inside the SQL builder.
type OutboxEventStatus string

const (
	// OutboxPending is the initial state, written inside the same transaction
	// as the fact the event describes.
	OutboxPending OutboxEventStatus = "PENDING"
	// OutboxProcessing means a worker has claimed the row. Claims are only
	// visible to the claimant; the pending query never returns them again.
	OutboxProcessing OutboxEventStatus = "PROCESSING"
	// OutboxProcessed is terminal: the side effect was delivered.
	OutboxProcessed OutboxEventStatus = "PROCESSED"
	// OutboxFailed is terminal: delivery was attempted and gave up.
	OutboxFailed OutboxEventStatus = "FAILED"
)

// IsTerminal reports whether the status ends the row's lifecycle. Only
// terminal statuses may be passed to a MarkProcessed call.
func (s OutboxEventStatus) IsTerminal() bool {
	return s == OutboxProcessed || s == OutboxFailed
}

// OutboxEvent is one claimed row of outbox_events.
//
// Payload stays opaque: it was serialised by the producer inside the article
// upsert transaction and is parsed by whichever consumer handles EventType.
// Nothing between those two points needs to look inside it.
type OutboxEvent struct {
	ID        string
	EventType string
	Payload   []byte
	Status    OutboxEventStatus
	CreatedAt time.Time
}
