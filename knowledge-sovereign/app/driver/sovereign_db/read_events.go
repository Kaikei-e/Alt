package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// KnowledgeEvent represents a single event in the knowledge event store.
type KnowledgeEvent struct {
	EventID       uuid.UUID
	EventSeq      int64
	OccurredAt    time.Time
	TenantID      uuid.UUID
	UserID        *uuid.UUID
	ActorType     string
	ActorID       string
	EventType     string
	AggregateType string
	AggregateID   string
	CorrelationID *uuid.UUID
	CausationID   *uuid.UUID
	DedupeKey     string
	Payload       json.RawMessage
}

// SequenceGapFrontier is what one snapshot says about a run of event_seq values
// missing from a projector's batch: whether the run is still empty, and the
// transaction ids that bound who could still fill it. It is what tells a hole
// that will still fill in apart from one that never will.
//
// All three come from a single statement, and that is the load-bearing part.
// Read Committed gives every statement its own snapshot — "two successive
// SELECT commands can see different data, even though they are within a single
// transaction, if other transactions commit changes after the first SELECT
// starts and before the second SELECT starts" (PostgreSQL 16 §13.2.1) — so a
// writer that commits between a batch read and a frontier read of its own is
// missing from the first and finished by the second, and a verdict drawn from
// that pair burns a sequence whose event is committed and sitting in the table.
// Asked in one statement, HoleOpen and Xmin are two facts about one snapshot:
// pg_current_snapshot returns the snapshot the statement's own scans are
// already reading under.
//
// Ceiling is the id PostgreSQL gave the reading transaction itself. Ids are
// handed out sequentially, at a transaction's first write, so "lower-numbered
// xids started writing before higher-numbered xids" (PostgreSQL 16 §74.1). A
// writer holding a sequence the batch did not see has written twice already —
// the dedupe row, then the event row that took the sequence — so its id is
// below any id handed out afterwards, Ceiling among them.
//
// That reasoning also reads the sequence backwards: a visible event_seq is
// taken to prove every lower value was handed out before it. Only a CACHE 1
// sequence says so — "with a cache setting greater than one you should only
// assume that the nextval values are all distinct, not that they are generated
// purely sequentially" (PostgreSQL 16, CREATE SEQUENCE). knowledge_events'
// BIGSERIAL is CACHE 1 and must stay there: raising it for append throughput
// lets a writer hold a low sequence it took after Ceiling was handed out, and
// this verdict then burns live events silently.
//
// Xmin is the oldest transaction still running when the snapshot was taken:
// "All transaction IDs less than xmin are either committed and visible, or
// rolled back and dead" (PostgreSQL 16, Table 9.81). An Xmin that has reached
// an earlier Ceiling therefore leaves a writer that holds one of these
// sequences only those two states, and HoleOpen — from the same snapshot —
// rules the first one out: committed and visible it is not, so it rolled back.
//
// The snapshot's xmax cannot stand in for Ceiling. It is "one past the highest
// completed transaction ID" — the highest id handed out is not bounded by it,
// and a writer that took the newest id sits at or above it, so a verdict drawn
// from xmax declares live writers finished and skips the sequences they hold.
// Nor can pg_snapshot_xip: it lists in-progress ids in [xmin, xmax) only, which
// is precisely the range such a writer is outside of.
type SequenceGapFrontier struct {
	Ceiling  int64
	Xmin     int64
	HoleOpen bool
}

// ReadSequenceGapFrontier reads the frontier for the run of sequences
// [firstSeq, lastSeq] — the ones missing from a projector's batch — in one
// statement, so the emptiness of the run and the ids that bound its possible
// writers describe the same snapshot. xid8 is a 64-bit counter that does not
// wrap, so both ids are comparable as plain integers.
//
// Reading this costs one transaction id, which is why projectors call it only
// while a hole actually blocks them: pg_current_xact_id_if_assigned would spend
// nothing, but a read-only transaction has no id and so yields no ceiling. The
// EXISTS adds an index probe per partition (idx_knowledge_events_seq).
func (r *Repository) ReadSequenceGapFrontier(ctx context.Context, firstSeq, lastSeq int64) (SequenceGapFrontier, error) {
	query := `SELECT pg_current_xact_id()::text::bigint,
		pg_snapshot_xmin(pg_current_snapshot())::text::bigint,
		NOT EXISTS (SELECT 1 FROM knowledge_events WHERE event_seq BETWEEN $1 AND $2)`
	var f SequenceGapFrontier
	if err := r.pool.QueryRow(ctx, query, firstSeq, lastSeq).Scan(&f.Ceiling, &f.Xmin, &f.HoleOpen); err != nil {
		return SequenceGapFrontier{}, fmt.Errorf("ReadSequenceGapFrontier: %w", err)
	}
	return f, nil
}

// ListKnowledgeEventsSince returns events after the given sequence number.
//
// event_seq is BIGSERIAL: the value is taken when a transaction inserts, not
// when it commits, so this returns a run with holes in it whenever a writer
// that took a lower sequence is still in flight. Callers that fold the result
// and move a cursor past it must advance only across the contiguous prefix —
// see usecase/projection_gap.
func (r *Repository) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]KnowledgeEvent, error) {
	query := `SELECT event_id, event_seq, occurred_at, tenant_id, user_id,
		actor_type, actor_id, event_type, aggregate_type, aggregate_id,
		correlation_id, causation_id, dedupe_key, payload
		FROM knowledge_events WHERE event_seq > $1
		ORDER BY event_seq ASC LIMIT $2`

	return r.scanEvents(ctx, query, afterSeq, limit)
}

// ListKnowledgeEventsSinceForUser returns events scoped to a tenant and user
// after the given sequence number. The tenant predicate is the primary
// authorization gate; user_id IS NULL admits tenant-wide system events
// (e.g., ArticleCreated) that all users in the tenant should observe.
func (r *Repository) ListKnowledgeEventsSinceForUser(ctx context.Context, tenantID, userID uuid.UUID, afterSeq int64, limit int) ([]KnowledgeEvent, error) {
	query := `SELECT event_id, event_seq, occurred_at, tenant_id, user_id,
		actor_type, actor_id, event_type, aggregate_type, aggregate_id,
		correlation_id, causation_id, dedupe_key, payload
		FROM knowledge_events
		WHERE event_seq > $1 AND tenant_id = $2 AND (user_id = $3 OR user_id IS NULL)
		ORDER BY event_seq ASC LIMIT $4`

	return r.scanEvents(ctx, query, afterSeq, tenantID, userID, limit)
}

// GetLatestKnowledgeEventSeqForUser returns the latest event sequence number
// observable by the given (tenant, user) pair.
func (r *Repository) GetLatestKnowledgeEventSeqForUser(ctx context.Context, tenantID, userID uuid.UUID) (int64, error) {
	query := `SELECT COALESCE(MAX(event_seq), 0) FROM knowledge_events
		WHERE tenant_id = $1 AND (user_id = $2 OR user_id IS NULL)`
	var seq int64
	if err := r.pool.QueryRow(ctx, query, tenantID, userID).Scan(&seq); err != nil {
		return 0, fmt.Errorf("GetLatestKnowledgeEventSeqForUser: %w", err)
	}
	return seq, nil
}

// AppendKnowledgeEvent inserts a knowledge event and returns its sequence, or
// 0 when the dedupe registry already held the key. This is the WIRE-level form:
// the AppendKnowledgeEvent RPC forwards the sequence verbatim and alt-backend's
// URL-backfill SkippedDuplicate counter (ADR-000869) reads seq > 0 as
// "genuinely appended" and seq == 0 as a dedupe hit, so the shape is pinned by
// the provider pact.
//
// In-process callers that act on the outcome (log it, count it, chain further
// work off it) MUST use AppendKnowledgeEventIfNew instead: a bare 0 is
// indistinguishable from "the caller never looked", which is exactly how the
// trail planner came to report every rejected re-proposal as an emission.
func (r *Repository) AppendKnowledgeEvent(ctx context.Context, event KnowledgeEvent) (int64, error) {
	seq, _, err := r.AppendKnowledgeEventIfNew(ctx, event)
	return seq, err
}

// AppendKnowledgeEventIfNew inserts a new knowledge event using a dedupe
// registry for idempotency, and reports whether the event was actually
// appended. The dedupe registry is a non-partitioned table that holds the
// global UNIQUE constraint on dedupe_key, which cannot be placed on the
// partitioned knowledge_events table directly (PostgreSQL requires partition
// key in all UNIQUE constraints).
//
// Both INSERTs run inside a single transaction: a crash between the two
// would otherwise leave the dedupe key registered with no corresponding
// event row, permanently losing the event (any resend is then treated as
// an already-applied duplicate and silently dropped). Their order carries a
// second guarantee: the dedupe INSERT gives the transaction its id before the
// event INSERT takes an event_seq, so a writer holding a sequence always has
// an id older than the sequence — see SequenceGapFrontier. An append that
// inserted the event row first would take its sequence before PostgreSQL had
// given it an id, and no ceiling read afterwards would stand above it.
//
// Flow:
//  1. INSERT into knowledge_event_dedupes (ON CONFLICT DO NOTHING)
//  2. If dedupe INSERT affected 0 rows → duplicate, report appended=false
//  3. Otherwise INSERT into knowledge_events and return event_seq
//  4. Commit both together
//
// A dedupe rejection is a normal outcome, not a failure — it is reported as
// (0, false, nil) rather than an error, so callers must branch on appended
// rather than on err. The compiler makes that branch impossible to skip.
func (r *Repository) AppendKnowledgeEventIfNew(ctx context.Context, event KnowledgeEvent) (int64, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("AppendKnowledgeEvent begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once Commit has succeeded

	// Step 1: Check dedupe registry
	dedupeQuery := `INSERT INTO knowledge_event_dedupes (dedupe_key, event_id, occurred_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (dedupe_key) DO NOTHING`

	tag, err := tx.Exec(ctx, dedupeQuery, event.DedupeKey, event.EventID, event.OccurredAt)
	if err != nil {
		return 0, false, fmt.Errorf("AppendKnowledgeEvent dedupe check: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return 0, false, nil // duplicate, idempotent — nothing to commit
	}

	// Step 2: Insert event
	eventQuery := `INSERT INTO knowledge_events
		(event_id, occurred_at, tenant_id, user_id, actor_type, actor_id,
		 event_type, aggregate_type, aggregate_id, correlation_id, causation_id,
		 dedupe_key, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING event_seq`

	var eventSeq int64
	err = tx.QueryRow(ctx, eventQuery,
		event.EventID, event.OccurredAt, event.TenantID, event.UserID,
		event.ActorType, event.ActorID, event.EventType,
		event.AggregateType, event.AggregateID,
		event.CorrelationID, event.CausationID,
		event.DedupeKey, event.Payload,
	).Scan(&eventSeq)
	if err != nil {
		return 0, false, fmt.Errorf("AppendKnowledgeEvent insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("AppendKnowledgeEvent commit: %w", err)
	}
	return eventSeq, true, nil
}

// ListKnowledgeEventsForUserInWindow returns events scoped strictly to the
// supplied user_id within [since, until), filtered by an event_type
// allowlist. Used by Surface Planner v2's resolver to pull cross-source
// evidence (recap topic snapshots / augur conversation links / summary
// version churn) without ever reading mutable views or latest state.
//
// F-001 mitigation: user_id is bound physically as a SQL parameter and
// the predicate is `user_id = $1` (no OR user_id IS NULL — system events
// never feed v2 evidence). Tenant-wide events would not be addressable to
// a single user's score window, and admitting them would let cross-tenant
// signals bleed into per-user placement.
//
// `eventTypes` is passed as a TEXT[] and matched with `= ANY(...)`. Empty
// slice returns no rows.
func (r *Repository) ListKnowledgeEventsForUserInWindow(
	ctx context.Context,
	userID uuid.UUID,
	eventTypes []string,
	since, until time.Time,
	limit int,
) ([]KnowledgeEvent, error) {
	if len(eventTypes) == 0 || limit <= 0 {
		return nil, nil
	}
	query := `SELECT event_id, event_seq, occurred_at, tenant_id, user_id,
		actor_type, actor_id, event_type, aggregate_type, aggregate_id,
		correlation_id, causation_id, dedupe_key, payload
		FROM knowledge_events
		WHERE user_id = $1
		  AND occurred_at >= $2
		  AND occurred_at < $3
		  AND event_type = ANY($4::text[])
		ORDER BY event_seq ASC LIMIT $5`

	return r.scanEvents(ctx, query, userID, since, until, eventTypes, limit)
}

// KnowledgeUserEvent represents a user interaction event (seen, opened, etc.).
type KnowledgeUserEvent struct {
	UserEventID uuid.UUID
	OccurredAt  time.Time
	UserID      uuid.UUID
	TenantID    uuid.UUID
	EventType   string
	ItemKey     string
	Payload     json.RawMessage
	DedupeKey   string
}

// AppendKnowledgeUserEvent inserts a user event with deduplication.
// Uses the unique index on (dedupe_key, occurred_at) for partitioned table compatibility.
// PostgreSQL requires ON CONFLICT columns to match a unique constraint exactly.
func (r *Repository) AppendKnowledgeUserEvent(ctx context.Context, event KnowledgeUserEvent) error {
	query := `INSERT INTO knowledge_user_events
		(user_event_id, occurred_at, user_id, tenant_id, event_type, item_key, payload, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (dedupe_key, occurred_at) WHERE dedupe_key != '' DO NOTHING`

	_, err := r.pool.Exec(ctx, query,
		event.UserEventID, event.OccurredAt, event.UserID, event.TenantID,
		event.EventType, event.ItemKey, event.Payload, event.DedupeKey,
	)
	if err != nil {
		return fmt.Errorf("AppendKnowledgeUserEvent: %w", err)
	}
	return nil
}

func (r *Repository) scanEvents(ctx context.Context, query string, args ...interface{}) ([]KnowledgeEvent, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("scanEvents: %w", err)
	}
	defer rows.Close()

	var events []KnowledgeEvent
	for rows.Next() {
		var e KnowledgeEvent
		if err := rows.Scan(
			&e.EventID, &e.EventSeq, &e.OccurredAt, &e.TenantID, &e.UserID,
			&e.ActorType, &e.ActorID, &e.EventType, &e.AggregateType, &e.AggregateID,
			&e.CorrelationID, &e.CausationID, &e.DedupeKey, &e.Payload,
		); err != nil {
			return nil, fmt.Errorf("scanEvents scan: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanEvents rows: %w", err)
	}
	return events, nil
}
