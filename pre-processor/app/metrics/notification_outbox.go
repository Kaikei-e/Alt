package metrics

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// OutboxRelayMetrics holds the two gauges the notification-outbox relay
// publishes, in the Prometheus exposition names every outbox producer shares.
//
// Both are written on every tick, including a backlog age of 0. A gauge that
// stops being written keeps reporting its last value, so a relay that wedges
// while the backlog was empty would otherwise look permanently healthy — and
// one that wedges while the backlog was deep would look permanently broken
// even after an operator drained it. Nothing is exposed before the first tick,
// so "the relay never ran" stays distinguishable from "the relay reports zero".
type OutboxRelayMetrics struct {
	mu                      sync.RWMutex
	ticked                  bool
	oldestPendingAgeSeconds float64
	lastTickUnixSeconds     float64
}

// NewOutboxRelayMetrics returns a metrics holder with no series exposed yet.
func NewOutboxRelayMetrics() *OutboxRelayMetrics {
	return &OutboxRelayMetrics{}
}

// ObserveTick records one relay tick. Callers must call it for every tick,
// successful or not: the last-tick gauge is a freshness signal, and a failed
// tick is still evidence the loop is alive.
func (m *OutboxRelayMetrics) ObserveTick(oldestPendingAge time.Duration, tickAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ticked = true
	m.oldestPendingAgeSeconds = oldestPendingAge.Seconds()
	m.lastTickUnixSeconds = float64(tickAt.UnixNano()) / float64(time.Second)
}

// Prometheus renders the gauges in text exposition format, or the empty string
// when no tick has been observed yet.
func (m *OutboxRelayMetrics) Prometheus() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.ticked {
		return ""
	}

	var b strings.Builder
	b.WriteString("# HELP notification_outbox_oldest_pending_age_seconds Age of the oldest notification_outbox row still awaiting forwarding.\n")
	b.WriteString("# TYPE notification_outbox_oldest_pending_age_seconds gauge\n")
	b.WriteString("notification_outbox_oldest_pending_age_seconds ")
	b.WriteString(formatGauge(m.oldestPendingAgeSeconds))
	b.WriteString("\n")
	b.WriteString("# HELP notification_outbox_last_tick_timestamp_seconds Unix time of the last notification-outbox relay tick.\n")
	b.WriteString("# TYPE notification_outbox_last_tick_timestamp_seconds gauge\n")
	b.WriteString("notification_outbox_last_tick_timestamp_seconds ")
	b.WriteString(formatGauge(m.lastTickUnixSeconds))
	b.WriteString("\n")
	return b.String()
}

func formatGauge(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
