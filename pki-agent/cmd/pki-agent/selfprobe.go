package main

import "time"

// selfProbeInterval is the cadence of the netns/listener self-probe. 30s
// matches Docker's default HEALTHCHECK interval and Kubernetes' typical
// periodSeconds (10s) on the slower side — frequent enough to close the
// netns-orphan detection window to ~90s, slow enough that a single netlink
// blip or cert-rotation restart window does not accumulate restarts.
const selfProbeInterval = 30 * time.Second

// probeFailureThreshold is the consecutive-failure count at which the
// self-probe loop triggers fail-fast exit, delegating recovery to Docker's
// `restart: unless-stopped` policy. Combined with selfProbeInterval this
// yields ~90s detection — within the industry-standard sidecar probe
// envelope (k8s default periodSeconds=10 × failureThreshold=3 = 30s; Dapr
// sidecar same defaults). See ADR-000802.
const probeFailureThreshold = 3

// probeState tracks consecutive self-probe failures so the tick loop can
// decide when to fail-fast.
//
// Kept as a plain struct (no mutex) because the tick loop runs in a single
// goroutine — the `select` in main's for-loop guarantees serial access.
type probeState struct {
	consecutive int
}

// evalProbeResult records a probe attempt and returns true when the caller
// should trigger fail-fast exit. Success resets the counter; an error
// increments it and signals exit once the threshold is reached.
func (s *probeState) evalProbeResult(err error) bool {
	if err == nil {
		s.consecutive = 0
		return false
	}
	s.consecutive++
	return s.consecutive >= probeFailureThreshold
}
