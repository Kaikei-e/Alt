package resilience

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelemetryHealth_ReportsExactlyOnceOnCrossingThreshold(t *testing.T) {
	var reports []TelemetryOutcome
	th := NewTelemetryHealth(TelemetryHealthConfig{
		DegradedThreshold: 3,
		OnChange:          func(o TelemetryOutcome) { reports = append(reports, o) },
	})

	th.RecordFailure("/x/Track")
	th.RecordFailure("/x/Track")
	require.Empty(t, reports, "below threshold must stay silent")

	th.RecordFailure("/x/Track")
	require.Len(t, reports, 1, "crossing the threshold must fire exactly one report")
	assert.True(t, reports[0].Degraded)
	assert.Equal(t, "/x/Track", reports[0].Endpoint)
	assert.Equal(t, 3, reports[0].ConsecFailures)
	assert.Equal(t, int64(3), reports[0].TotalFailures)
}

func TestTelemetryHealth_DoesNotReportAgainWhileStillDegraded(t *testing.T) {
	var reports []TelemetryOutcome
	th := NewTelemetryHealth(TelemetryHealthConfig{
		DegradedThreshold: 2,
		OnChange:          func(o TelemetryOutcome) { reports = append(reports, o) },
	})

	for i := 0; i < 10; i++ {
		th.RecordFailure("/x/Track")
	}

	require.Len(t, reports, 1,
		"a 100%%-failing endpoint must log once, not once per request")
}

func TestTelemetryHealth_ReportsRecoveryExactlyOnce(t *testing.T) {
	var reports []TelemetryOutcome
	th := NewTelemetryHealth(TelemetryHealthConfig{
		DegradedThreshold: 2,
		OnChange:          func(o TelemetryOutcome) { reports = append(reports, o) },
	})

	th.RecordFailure("/x/Track")
	th.RecordFailure("/x/Track")
	require.Len(t, reports, 1)

	th.RecordSuccess("/x/Track")
	require.Len(t, reports, 2, "the first success after degraded must report recovery")
	assert.False(t, reports[1].Degraded)

	th.RecordSuccess("/x/Track")
	require.Len(t, reports, 2, "further successes while already healthy must not report again")
}

func TestTelemetryHealth_TracksEndpointsIndependently(t *testing.T) {
	var reports []TelemetryOutcome
	th := NewTelemetryHealth(TelemetryHealthConfig{
		DegradedThreshold: 2,
		OnChange:          func(o TelemetryOutcome) { reports = append(reports, o) },
	})

	th.RecordFailure("/x/TrackHomeAction")
	th.RecordFailure("/x/TrackHomeAction")
	require.Len(t, reports, 1)
	assert.Equal(t, "/x/TrackHomeAction", reports[0].Endpoint)

	// A different endpoint succeeding must not be conflated with the failing one.
	th.RecordSuccess("/x/EmitTrailOutcome")
	require.Len(t, reports, 1, "an unrelated endpoint's success must not report EmitTrailOutcome as recovered")
}

func TestTelemetryHealth_StatsAggregatesAcrossEndpoints(t *testing.T) {
	th := NewTelemetryHealth(TelemetryHealthConfig{DegradedThreshold: 5})

	th.RecordSuccess("/x/A")
	th.RecordSuccess("/x/A")
	th.RecordFailure("/x/B")

	stats := th.Stats()
	assert.Equal(t, int64(2), stats.TotalSuccesses)
	assert.Equal(t, int64(1), stats.TotalFailures)
}

func TestTelemetryHealth_NilOnChangeDoesNotPanic(t *testing.T) {
	th := NewTelemetryHealth(TelemetryHealthConfig{DegradedThreshold: 1})

	assert.NotPanics(t, func() {
		th.RecordFailure("/x/Track")
		th.RecordSuccess("/x/Track")
	})
}
