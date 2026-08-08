package resilience

import "sync"

// TelemetryOutcome is a degraded/recovered report for one ClassTelemetry
// endpoint. ClassTelemetry RPCs are exempt from circuit-breaker gating --
// they are fire-and-forget writes the caller discards, so refusing them buys
// nothing -- but exempt from gating must not mean exempt from observability:
// a run of failures on one must still surface, or a broken RPC becomes
// indistinguishable from a healthy but unused one (adversarial review).
type TelemetryOutcome struct {
	Endpoint string
	// Degraded is true when this report is "just crossed into failing" and
	// false when it is "just recovered".
	Degraded       bool
	ConsecFailures int
	TotalFailures  int64
	TotalSuccesses int64
}

// TelemetryHealthConfig configures TelemetryHealth.
type TelemetryHealthConfig struct {
	// DegradedThreshold is the number of consecutive failures, on one
	// endpoint, before a degraded report fires. Mirrors the other classes'
	// FailureThreshold, but as a reporting cadence rather than an admission
	// budget: a transient blip below this stays silent.
	DegradedThreshold int
	// OnChange receives a report only on the two edges an operator needs to
	// act on: crossing into degraded, and the first success after. Left nil,
	// TelemetryHealth still counts outcomes but never reports them --
	// production wiring must always set it.
	OnChange func(TelemetryOutcome)
}

type telemetryCounters struct {
	consecFailures int
	totalFailures  int64
	totalSuccesses int64
	degraded       bool
}

// TelemetryHealth tracks per-endpoint outcome counters for ClassTelemetry
// RPCs. It never gates admission -- there is no Acquire -- it only counts and
// reports.
type TelemetryHealth struct {
	mu         sync.Mutex
	cfg        TelemetryHealthConfig
	byEndpoint map[string]*telemetryCounters
}

// NewTelemetryHealth creates a TelemetryHealth tracker.
func NewTelemetryHealth(cfg TelemetryHealthConfig) *TelemetryHealth {
	return &TelemetryHealth{cfg: cfg, byEndpoint: make(map[string]*telemetryCounters)}
}

// RecordSuccess records a successful outcome for endpoint. If the endpoint
// was degraded, this fires exactly one recovered report on OnChange.
func (t *TelemetryHealth) RecordSuccess(endpoint string) {
	t.mu.Lock()
	c := t.entryLocked(endpoint)
	c.totalSuccesses++
	c.consecFailures = 0
	wasDegraded := c.degraded
	c.degraded = false
	out := TelemetryOutcome{
		Endpoint:       endpoint,
		Degraded:       false,
		ConsecFailures: c.consecFailures,
		TotalFailures:  c.totalFailures,
		TotalSuccesses: c.totalSuccesses,
	}
	t.mu.Unlock()

	if wasDegraded && t.cfg.OnChange != nil {
		t.cfg.OnChange(out)
	}
}

// RecordFailure records a failed outcome for endpoint. It fires exactly one
// degraded report on OnChange the call that makes ConsecFailures cross
// DegradedThreshold, not on every failing call after.
func (t *TelemetryHealth) RecordFailure(endpoint string) {
	t.mu.Lock()
	c := t.entryLocked(endpoint)
	c.totalFailures++
	c.consecFailures++
	justDegraded := !c.degraded && c.consecFailures >= t.cfg.DegradedThreshold
	if justDegraded {
		c.degraded = true
	}
	out := TelemetryOutcome{
		Endpoint:       endpoint,
		Degraded:       true,
		ConsecFailures: c.consecFailures,
		TotalFailures:  c.totalFailures,
		TotalSuccesses: c.totalSuccesses,
	}
	t.mu.Unlock()

	if justDegraded && t.cfg.OnChange != nil {
		t.cfg.OnChange(out)
	}
}

func (t *TelemetryHealth) entryLocked(endpoint string) *telemetryCounters {
	c, ok := t.byEndpoint[endpoint]
	if !ok {
		c = &telemetryCounters{}
		t.byEndpoint[endpoint] = c
	}
	return c
}

// TelemetryHealthStats is the aggregate totals across every endpoint this
// tracker has seen, for /v1/bff/stats. Unlike CircuitBreakerStats there is no
// State: ClassTelemetry endpoints are never gated, so there is no open/closed
// admission state to report.
type TelemetryHealthStats struct {
	TotalSuccesses int64
	TotalFailures  int64
}

// Stats returns the current aggregate totals.
func (t *TelemetryHealth) Stats() TelemetryHealthStats {
	t.mu.Lock()
	defer t.mu.Unlock()

	var out TelemetryHealthStats
	for _, c := range t.byEndpoint {
		out.TotalSuccesses += c.totalSuccesses
		out.TotalFailures += c.totalFailures
	}
	return out
}
