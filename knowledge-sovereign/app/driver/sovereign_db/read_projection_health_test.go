package sovereign_db

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetKnowledgeEventLastOccurrenceAges_PinsGaugeSemantics locks the three
// cases the producer-liveness alert is built on. These are the meanings
// ADR-000939 depends on and no query rewrite may change them:
//   - a recent producer reports its exact age,
//   - a stale producer reports its exact age however old it is (no clamping to
//     a lookback window — a 40-day-old SummarySuperseded must still read 40
//     days, not "at least N"),
//   - a producer that has never emitted is ABSENT from the map, which is what
//     the exporter turns into the 10-year never-seen sentinel.
func TestGetKnowledgeEventLastOccurrenceAges_PinsGaugeSemantics(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	watched := []string{
		"recap.topic_snapshotted.v1", // never seen — the PM-2026-045 producer
		"SummarySuperseded",          // stale — last emitted 40 days ago
		"SummaryVersionCreated",      // recent — the "pipeline alive" reference
	}
	mock.ExpectQuery(regexp.QuoteMeta(knowledgeEventLastOccurrenceAgesQuery)).
		WithArgs(watched).
		WillReturnRows(pgxmock.NewRows([]string{"event_type", "age_seconds"}).
			AddRow("SummarySuperseded", 3_456_000.0).
			AddRow("SummaryVersionCreated", 30.0))

	ages, err := repo.GetKnowledgeEventLastOccurrenceAges(context.Background(), watched)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	assert.Equal(t, map[string]float64{
		"SummarySuperseded":     3_456_000,
		"SummaryVersionCreated": 30,
	}, ages)

	// The never-seen producer must stay absent. Materialising it as 0 would
	// read as "freshly alive" and silently disarm the only gate that catches a
	// producer that never wired up.
	_, ok := ages["recap.topic_snapshotted.v1"]
	assert.False(t, ok,
		"a never-emitted producer must be omitted so the exporter publishes the 10-year sentinel")
}

// TestGetKnowledgeEventLastOccurrenceAges_EmptyRequestSkipsTheDatabase keeps
// the short-circuit: no watched types means no query at all.
func TestGetKnowledgeEventLastOccurrenceAges_EmptyRequestSkipsTheDatabase(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &Repository{pool: mock}

	ages, err := repo.GetKnowledgeEventLastOccurrenceAges(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, ages)
	// No expectation was set — reaching the pool would fail the mock.
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGetKnowledgeEventLastOccurrenceAges_DoesNotAggregateWholeEventLog is the
// regression guard for the 60-second full-scan.
//
// The gauge used to be `max(occurred_at) ... GROUP BY event_type`. PostgreSQL
// cannot apply its MIN/MAX index optimisation to that shape:
// src/backend/optimizer/plan/planagg.c, preprocess_minmax_aggregates, bails out
// with `if (parse->groupClause || list_length(parse->groupingSets) > 1 ||
// parse->hasWindowFuncs) return;` — "We don't handle GROUP BY or windowing,
// because our current implementations of grouping require looking at all the
// rows anyway". So every tick read every row of every watched type (~1.8M heap
// fetches at six months of traffic, spread over all partitions including the
// oversized default one) and evicted the shared_buffers pages the interactive
// Home/Trail read path needs.
//
// The cheap shape is one ordered index descent per watched type: an aggregate-
// free `ORDER BY occurred_at DESC LIMIT 1` that idx_knowledge_events_type_occurred
// satisfies with a MergeAppend of per-partition index scans, each stopped after
// its first row.
func TestGetKnowledgeEventLastOccurrenceAges_DoesNotAggregateWholeEventLog(t *testing.T) {
	q := knowledgeEventLastOccurrenceAgesQuery
	upper := strings.ToUpper(q)

	assert.NotContains(t, upper, "GROUP BY",
		"GROUP BY disables planagg.c's MIN/MAX optimisation and forces a full scan of every watched type every tick")
	assert.NotContains(t, upper, "MAX(",
		"a per-type max() aggregate reads every row of that type; use ORDER BY ... LIMIT 1 so an index can stop at the first")

	assert.Contains(t, q, "ORDER BY ke.occurred_at DESC",
		"the per-type probe must be ordered so the index supplies the newest row first")
	assert.Contains(t, q, "LIMIT 1",
		"the per-type probe must stop after the newest row")
}

// TestKnowledgeEventsTypeOccurredIndexExists couples the query shape above to
// the index that makes it cheap. Without (event_type, occurred_at DESC) the
// ordered LIMIT 1 has to walk idx_knowledge_events_occurred backwards filtering
// on event_type, which for a dead producer — exactly the case the gauge exists
// to catch — degrades to the full scan this change removed.
func TestKnowledgeEventsTypeOccurredIndexExists(t *testing.T) {
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations",
		"00030_add_event_type_occurred_index_for_liveness_gauge.sql"))
	require.NoError(t, err)

	assert.Contains(t, string(sql), "ON knowledge_events (event_type, occurred_at DESC)",
		"the producer-liveness probe needs (event_type, occurred_at DESC) to resolve with an index descent")
}
