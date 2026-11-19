package sovereign_db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ReadingSource is one article that counted as read for a user, projected from
// reading.session_ended.v1 (or legacy HomeItemOpened) events.
type ReadingSource struct {
	UserID          uuid.UUID
	TenantID        uuid.UUID
	ArticleID       uuid.UUID
	FirstReadAt     time.Time
	ReadRuleVersion int
}

// ReadingQueueItem is one entry in the user's Read Today queue read model.
type ReadingQueueItem struct {
	UserID             uuid.UUID
	TenantID           uuid.UUID
	ArticleID          uuid.UUID
	QueuedAt           time.Time
	ExpiresAt          time.Time
	QueuePolicyVersion int
	LastEventSeq       int64
}

// UpsertReadingSource records or updates a reading source idempotently.
// Merge rule (order-independent and idempotent under replay):
//   - incoming read_rule_version > stored: take incoming first_read_at and version;
//   - incoming read_rule_version < stored: keep stored row unchanged;
//   - equal read_rule_version: keep LEAST(stored.first_read_at, incoming.first_read_at).
func (r *Repository) UpsertReadingSource(ctx context.Context, src ReadingSource) error {
	const q = `
INSERT INTO reading_sources (user_id, tenant_id, article_id, first_read_at, read_rule_version)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, article_id) DO UPDATE SET
  first_read_at = CASE
    WHEN EXCLUDED.read_rule_version > reading_sources.read_rule_version THEN EXCLUDED.first_read_at
    WHEN EXCLUDED.read_rule_version < reading_sources.read_rule_version THEN reading_sources.first_read_at
    ELSE LEAST(reading_sources.first_read_at, EXCLUDED.first_read_at)
  END,
  read_rule_version = GREATEST(reading_sources.read_rule_version, EXCLUDED.read_rule_version)`
	if _, err := r.pool.Exec(ctx, q,
		src.UserID, src.TenantID, src.ArticleID, src.FirstReadAt, src.ReadRuleVersion,
	); err != nil {
		return fmt.Errorf("UpsertReadingSource: %w", err)
	}
	return nil
}

// UpsertReadingQueueItem inserts or re-activates a reading queue entry when the
// folding event's sequence is at least as recent as the last folded sequence.
// An active, non-expired row (as of the event's queued_at) preserves its
// original queue window so duplicate adds do not extend the TTL.
func (r *Repository) UpsertReadingQueueItem(ctx context.Context, item ReadingQueueItem) error {
	const q = `
INSERT INTO reading_queue_items (
  user_id, tenant_id, article_id, queued_at, expires_at, queue_policy_version,
  removed_at, removed_reason, last_event_seq
) VALUES ($1, $2, $3, $4, $5, $6, NULL, '', $7)
ON CONFLICT (user_id, article_id) DO UPDATE SET
  queued_at = CASE
    WHEN reading_queue_items.removed_at IS NULL AND reading_queue_items.expires_at > EXCLUDED.queued_at
      THEN reading_queue_items.queued_at
    ELSE EXCLUDED.queued_at
  END,
  expires_at = CASE
    WHEN reading_queue_items.removed_at IS NULL AND reading_queue_items.expires_at > EXCLUDED.queued_at
      THEN reading_queue_items.expires_at
    ELSE EXCLUDED.expires_at
  END,
  queue_policy_version = CASE
    WHEN reading_queue_items.removed_at IS NULL AND reading_queue_items.expires_at > EXCLUDED.queued_at
      THEN reading_queue_items.queue_policy_version
    ELSE EXCLUDED.queue_policy_version
  END,
  removed_at     = NULL,
  removed_reason = '',
  last_event_seq = EXCLUDED.last_event_seq
WHERE reading_queue_items.last_event_seq <= EXCLUDED.last_event_seq`
	if _, err := r.pool.Exec(ctx, q,
		item.UserID, item.TenantID, item.ArticleID,
		item.QueuedAt, item.ExpiresAt, item.QueuePolicyVersion, item.LastEventSeq,
	); err != nil {
		return fmt.Errorf("UpsertReadingQueueItem: %w", err)
	}
	return nil
}

// RemoveReadingQueueItem marks an active queue entry removed when the article is
// explicitly unqueued or becomes read.
func (r *Repository) RemoveReadingQueueItem(
	ctx context.Context,
	userID, articleID uuid.UUID,
	removedAt time.Time,
	reason string,
	eventSeq int64,
) error {
	const q = `
UPDATE reading_queue_items
SET removed_at     = $3,
    removed_reason = $4,
    last_event_seq = $5
WHERE user_id = $1
  AND article_id = $2
  AND removed_at IS NULL
  AND last_event_seq <= $5`
	if _, err := r.pool.Exec(ctx, q, userID, articleID, removedAt, reason, eventSeq); err != nil {
		return fmt.Errorf("RemoveReadingQueueItem: %w", err)
	}
	return nil
}

// GetActiveReadingQueue returns active, non-expired queue items as of asOf,
// ordered oldest-queued first.
func (r *Repository) GetActiveReadingQueue(ctx context.Context, userID uuid.UUID, asOf time.Time) ([]ReadingQueueItem, error) {
	const q = `
SELECT user_id, tenant_id, article_id, queued_at, expires_at, queue_policy_version, last_event_seq
FROM reading_queue_items
WHERE user_id = $1
  AND removed_at IS NULL
  AND expires_at > $2
ORDER BY queued_at ASC, article_id ASC`
	rows, err := r.pool.Query(ctx, q, userID, asOf)
	if err != nil {
		return nil, fmt.Errorf("GetActiveReadingQueue: %w", err)
	}
	defer rows.Close()

	var items []ReadingQueueItem
	for rows.Next() {
		var it ReadingQueueItem
		if err := rows.Scan(
			&it.UserID, &it.TenantID, &it.ArticleID,
			&it.QueuedAt, &it.ExpiresAt, &it.QueuePolicyVersion, &it.LastEventSeq,
		); err != nil {
			return nil, fmt.Errorf("GetActiveReadingQueue scan: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetActiveReadingQueue rows: %w", err)
	}
	return items, nil
}

// CountReadingSourcesSince returns the count of distinct articles that counted
// as read under rule version >= 1 with first_read_at >= since.
func (r *Repository) CountReadingSourcesSince(ctx context.Context, userID uuid.UUID, since time.Time) (int, error) {
	const q = `
SELECT COUNT(*)
FROM reading_sources
WHERE user_id = $1
  AND first_read_at >= $2
  AND read_rule_version >= 1`
	var count int
	if err := r.pool.QueryRow(ctx, q, userID, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("CountReadingSourcesSince: %w", err)
	}
	return count, nil
}
