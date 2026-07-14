package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FetchFeedTitlesByIDs returns feed_id -> feed.title for all feed_ids in a
// single round-trip. Used by the Morning Letter enrichment path to attach
// a feed byline to each bullet card.
//
// Unknown ids are silently omitted from the map — callers should default
// to an empty string when the lookup misses.
func (r *AltDBRepository) FetchFeedTitlesByIDs(
	ctx context.Context,
	feedIDs []uuid.UUID,
) (map[uuid.UUID]string, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}
	if len(feedIDs) == 0 {
		return map[uuid.UUID]string{}, nil
	}

	// Same pattern as FetchArticlesByIDs: pass []string to keep PgBouncer
	// simple protocol happy with uuid[] binding.
	stringIDs := make([]string, len(feedIDs))
	for i, id := range feedIDs {
		stringIDs[i] = id.String()
	}

	rows, err := r.pool.Query(ctx,
		"SELECT id, title FROM feeds WHERE id = ANY($1::uuid[])",
		stringIDs,
	)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "FetchFeedTitlesByIDs query failed", "error", err)
		return nil, errors.New("error querying feed titles")
	}
	defer rows.Close()

	out := make(map[uuid.UUID]string, len(feedIDs))
	for rows.Next() {
		var id uuid.UUID
		var title string
		if err := rows.Scan(&id, &title); err != nil {
			logger.Logger.ErrorContext(ctx, "FetchFeedTitlesByIDs scan failed", "error", err)
			return nil, errors.New("error scanning feed title row")
		}
		out[id] = title
	}
	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "FetchFeedTitlesByIDs rows.Err", "error", err)
		return nil, errors.New("error iterating feed title rows")
	}
	return out, nil
}
