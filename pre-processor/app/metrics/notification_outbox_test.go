package metrics

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutboxRelayMetrics_EmitsNothingBeforeTheFirstTick(t *testing.T) {
	m := NewOutboxRelayMetrics()

	assert.Empty(t, m.Prometheus(),
		"an absent series is how a relay that never ran is distinguishable from one reporting zero")
}

func TestOutboxRelayMetrics_WritesBothGaugesOnEveryTick(t *testing.T) {
	m := NewOutboxRelayMetrics()
	tickAt := time.Date(2026, 8, 8, 11, 0, 0, 0, time.UTC)

	m.ObserveTick(90*time.Second, tickAt)
	first := m.Prometheus()

	require.Contains(t, first, "notification_outbox_oldest_pending_age_seconds 90")
	require.Contains(t, first, "# TYPE notification_outbox_oldest_pending_age_seconds gauge")
	require.Contains(t, first, "# TYPE notification_outbox_last_tick_timestamp_seconds gauge")
	require.Contains(t, first, "notification_outbox_last_tick_timestamp_seconds "+unixSecondsLiteral(tickAt))

	// The backlog drained: the gauge must be written down to 0, not left
	// reporting the previous value.
	nextTick := tickAt.Add(15 * time.Second)
	m.ObserveTick(0, nextTick)
	second := m.Prometheus()

	assert.Contains(t, second, "notification_outbox_oldest_pending_age_seconds 0")
	assert.NotContains(t, second, "notification_outbox_oldest_pending_age_seconds 90")
	assert.Contains(t, second, "notification_outbox_last_tick_timestamp_seconds "+unixSecondsLiteral(nextTick))
}

func unixSecondsLiteral(at time.Time) string {
	return strconv.FormatFloat(float64(at.UnixNano())/float64(time.Second), 'f', -1, 64)
}
