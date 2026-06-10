package sovereign_db

import (
	"context"
	"fmt"
)

// Projection-health read queries backing the ADR-000939 honest gate. These are
// DB-truth gauges (sampled periodically), not rate counters — they evaluate
// even at the low projection traffic (~250 entries/day) that made the old
// rate-based coverage alert structurally unable to fire.

// KnowledgeLoopRelationCoverage is the relation-coverage snapshot over recently
// sourced entries. WithRelations / Total is the honest "is evidence reaching
// the Orient surface" ratio.
type KnowledgeLoopRelationCoverage struct {
	Total         int64
	WithRelations int64
}

// GetKnowledgeLoopRelationCoverage24h counts entries whose source event
// occurred in the last 24h (freshness_at, event-time — NOT projected_at, which
// is debug-only and not metric-exposed) and how many carry a non-empty
// relation-set. A collapse to ~0 while entries keep flowing is the "decorated
// feed" regression (ADR-000938/000939): a producer feeding the accumulator died
// or extractRelations dropped the evidence.
func (r *Repository) GetKnowledgeLoopRelationCoverage24h(ctx context.Context) (KnowledgeLoopRelationCoverage, error) {
	const q = `
SELECT count(*) AS total,
       count(*) FILTER (
         WHERE relations IS NOT NULL AND relations::text NOT IN ('[]', 'null')
       ) AS with_relations
FROM knowledge_loop_entries
WHERE freshness_at >= now() - interval '24 hours'`
	var out KnowledgeLoopRelationCoverage
	if err := r.pool.QueryRow(ctx, q).Scan(&out.Total, &out.WithRelations); err != nil {
		return out, fmt.Errorf("GetKnowledgeLoopRelationCoverage24h: %w", err)
	}
	return out, nil
}

// GetKnowledgeEventLastOccurrenceAges returns, per requested event_type, the
// age in seconds of the most recent event of that type (now() - max(occurred_at)).
// Event types with no rows are omitted from the map — the caller decides how to
// represent "never seen" (the exporter publishes a large sentinel age so a
// producer that has never emitted is visibly stale rather than absent).
//
// This is the producer-liveness signal: a recap.topic_snapshotted.v1 /
// augur.conversation_linked.v1 age that climbs while the rest of the pipeline
// stays fresh distinguishes "the producer died" from "no usage".
func (r *Repository) GetKnowledgeEventLastOccurrenceAges(ctx context.Context, eventTypes []string) (map[string]float64, error) {
	if len(eventTypes) == 0 {
		return map[string]float64{}, nil
	}
	const q = `
SELECT event_type, EXTRACT(EPOCH FROM (now() - max(occurred_at)))::float8 AS age_seconds
FROM knowledge_events
WHERE event_type = ANY($1::text[])
GROUP BY event_type`
	rows, err := r.pool.Query(ctx, q, eventTypes)
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
