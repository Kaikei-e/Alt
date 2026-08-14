package sovereign_db

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GetProjectionLag feeds a field the wire declares as seconds
// (GetProjectionLagResponse.lag_seconds, double), which alt-backend converts
// straight into a time.Duration of seconds and alerts on at
// alt_home_projector_lag_seconds > 600 / > 1800. So the query has to return a
// duration, not a count of events: max(event_seq) - min(checkpoint) is an event
// backlog, and the two units do not even correlate — a healthy projector 700
// events behind pages, an hour-dead one 100 events behind stays silent.
//
// The duration that means "the projection is behind" is how long the oldest
// event nobody has folded yet has been waiting, so the query must reach
// knowledge_events.occurred_at and subtract it from now().
func TestGetProjectionLag_ReturnsSecondsNotAnEventCount(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, err := repo.GetProjectionLag(context.Background())
	require.NoError(t, err)
	require.Len(t, mock.queryRowCalls, 1)

	sql := mock.queryRowCalls[0].SQL
	assert.Contains(t, sql, "EXTRACT(EPOCH",
		"lag_seconds must be a wall-clock duration; a bare sequence subtraction reports events in a field alerted on as seconds")
	assert.Contains(t, sql, "occurred_at",
		"the duration is measured from the oldest unprojected event's occurred_at")
	assert.NotContains(t, sql, "MAX(event_seq)",
		"max(event_seq) - min(checkpoint) is the event-count form this field must never carry again")
}

// The MIN over the checkpoint table is taken across every row it finds, and
// retired projectors keep their rows: migration 00028 dropped the Knowledge Loop
// read models but not the `knowledge-loop-projector` / `surface_planner_v2`
// checkpoints the loop runbook documents. A frozen row like that is the
// permanent minimum, so the reported lag grows forever no matter how current the
// projectors that are actually running are.
//
// Scoping the MIN to the in-process projector roster is what keeps the gauge
// describing live projectors. Binding the roster (rather than filtering on a
// name pattern) also means retiring a projector is a one-line registry edit.
func TestGetProjectionLag_MeasuresOnlyTheProjectorsThatAreRunning(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, err := repo.GetProjectionLag(context.Background())
	require.NoError(t, err)
	require.Len(t, mock.queryRowCalls, 1)

	call := mock.queryRowCalls[0]
	require.Len(t, call.Args, 1, "the live projector roster must be bound, or the MIN spans retired checkpoints")
	names, ok := call.Args[0].([]string)
	require.True(t, ok, "the roster is bound as a text[]")

	want := []string{}
	for _, target := range RebuildTargets() {
		want = append(want, target.ProjectorName())
	}
	assert.ElementsMatch(t, want, names,
		"the roster must be the in-process projectors, so a retired projector's frozen checkpoint cannot pin the minimum")

	assert.Contains(t, call.SQL, "LEFT JOIN knowledge_projection_checkpoints",
		"a projector that has never run has no row at all; an inner join would drop it from the MIN and hide the worst lag there is")
}

// The scanned value is the answer, in seconds, verbatim: the RPC forwards it to
// alt-backend, which multiplies by time.Second. Anything applied on top here
// would be a second unit conversion nobody downstream knows about.
func TestGetProjectionLag_ForwardsTheScannedSecondsVerbatim(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			require.Len(t, dest, 1)
			seconds, ok := dest[0].(*float64)
			require.True(t, ok, "lag is scanned as float8 seconds")
			*seconds = 742.5
			return nil
		}}
	}
	repo := &Repository{pool: mock}

	lag, err := repo.GetProjectionLag(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 742.5, lag, 0.0001)
}

// A caught-up projection has no unprojected event to measure, and the answer
// then is 0 seconds rather than NULL — the scan destination is a plain float64,
// so a NULL would fail the whole health read instead of reporting "no lag".
func TestGetProjectionLag_CaughtUpProjectionIsZeroNotNull(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	_, err := repo.GetProjectionLag(context.Background())
	require.NoError(t, err)
	require.Len(t, mock.queryRowCalls, 1)

	sql := mock.queryRowCalls[0].SQL
	assert.True(t, strings.Contains(sql, "COALESCE"),
		"the no-unprojected-event case must resolve to 0, not NULL, or a healthy projection breaks the scan")
	assert.Contains(t, sql, "GREATEST",
		"a future-dated event must not report negative lag; alt-backend reads negatives as its unavailable sentinel")
}
