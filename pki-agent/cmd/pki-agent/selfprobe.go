package main

// probeFailureThreshold is the consecutive-failure count at which the tick
// loop exits, delegating recovery to Docker's `restart: unless-stopped`
// policy. With the default 5 min tick this gives ~15 min of tolerance for
// transient errors (step-ca blip, netlink stall, cert rotation overlap)
// before treating the condition as structural. See ADR-000784.
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
