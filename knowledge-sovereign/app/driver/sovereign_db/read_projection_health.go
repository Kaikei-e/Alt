package sovereign_db

import (
	"context"
	"fmt"
)

// Projection-health read queries backing the producer-liveness gauges. These
// are DB-truth gauges (sampled periodically), not rate counters — they evaluate
// even at low projection traffic.

// knowledgeEventLastOccurrenceAgesQuery runs one ordered probe per requested
// type instead of max(occurred_at) GROUP BY event_type. The aggregate form
// cannot use PostgreSQL's MIN/MAX index optimisation — planagg.c's
// preprocess_minmax_aggregates returns early on any groupClause ("our current
// implementations of grouping require looking at all the rows anyway") — so it
// read every row of every watched type on each tick and evicted the
// shared_buffers pages the Home/Trail read path needs.
//
// The LATERAL keeps the aggregate out of the picture: each type resolves to a
// MergeAppend over per-partition idx_knowledge_events_type_occurred scans
// (migration 00030), stopped after the first row. A type with no rows anywhere
// contributes no row (CROSS JOIN, not LEFT JOIN) and stays omitted from the
// result, so the caller's never-seen sentinel path is unchanged. Ages stay
// exact at any age; there is no lookback window to clamp them to.
const knowledgeEventLastOccurrenceAgesQuery = `
SELECT t.event_type,
       EXTRACT(EPOCH FROM (now() - latest.occurred_at))::float8 AS age_seconds
FROM unnest($1::text[]) AS t(event_type)
CROSS JOIN LATERAL (
  SELECT ke.occurred_at
  FROM knowledge_events ke
  WHERE ke.event_type = t.event_type
  ORDER BY ke.occurred_at DESC
  LIMIT 1
) AS latest`

// GetKnowledgeEventLastOccurrenceAges returns, per requested event_type, the
// age in seconds of the most recent event of that type (now() - newest
// occurred_at). Event types with no rows are omitted from the map — the caller
// decides how to represent "never seen" (the exporter publishes a large
// sentinel age so a producer that has never emitted is visibly stale rather
// than absent).
//
// This is the producer-liveness signal: a recap.topic_snapshotted.v1 /
// augur.conversation_linked.v1 age that climbs while the rest of the pipeline
// stays fresh distinguishes "the producer died" from "no usage".
func (r *Repository) GetKnowledgeEventLastOccurrenceAges(ctx context.Context, eventTypes []string) (map[string]float64, error) {
	if len(eventTypes) == 0 {
		return map[string]float64{}, nil
	}
	rows, err := r.pool.Query(ctx, knowledgeEventLastOccurrenceAgesQuery, eventTypes)
	if err != nil {
		return nil, fmt.Errorf("GetKnowledgeEventLastOccurrenceAges query: %w", err)
	}
	defer rows.Close()

	out := make(map[string]float64, len(eventTypes))
	for rows.Next() {
		var (
			etype string
			age   float64
		)
		if err := rows.Scan(&etype, &age); err != nil {
			return nil, fmt.Errorf("GetKnowledgeEventLastOccurrenceAges scan: %w", err)
		}
		out[etype] = age
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetKnowledgeEventLastOccurrenceAges rows: %w", err)
	}
	return out, nil
}
