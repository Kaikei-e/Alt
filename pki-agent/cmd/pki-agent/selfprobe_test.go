package main

import (
	"errors"
	"testing"
	"time"
)

// TestProbeState_SuccessDoesNotExit: a clean probe must never trigger exit,
// and must leave the counter at zero.
func TestProbeState_SuccessDoesNotExit(t *testing.T) {
	var s probeState
	if s.evalProbeResult(nil) {
		t.Fatal("want no-exit on success, got exit")
	}
	if s.consecutive != 0 {
		t.Fatalf("want counter=0 on success, got %d", s.consecutive)
	}
}

// TestProbeState_SingleFailureDoesNotExit: one transient error (cert rotation
// crossing a tick boundary, short netlink blip) must not kill the container.
func TestProbeState_SingleFailureDoesNotExit(t *testing.T) {
	var s probeState
	if s.evalProbeResult(errors.New("transient")) {
		t.Fatal("want no-exit on single failure, got exit")
	}
	if s.consecutive != 1 {
		t.Fatalf("want counter=1, got %d", s.consecutive)
	}
}

// TestProbeState_TwoFailuresDoNotExit: the threshold is 3 — two is still
// within the bounce budget.
func TestProbeState_TwoFailuresDoNotExit(t *testing.T) {
	var s probeState
	s.evalProbeResult(errors.New("e1"))
	if s.evalProbeResult(errors.New("e2")) {
		t.Fatal("want no-exit at count=2, got exit")
	}
	if s.consecutive != 2 {
		t.Fatalf("want counter=2, got %d", s.consecutive)
	}
}

// TestProbeState_ThreeFailuresTriggerExit: sustained failure (15 min with
// the default 5 min tick) is the signal for netns-orphan — exit so compose
// restarts the container in the current parent netns.
func TestProbeState_ThreeFailuresTriggerExit(t *testing.T) {
	var s probeState
	s.evalProbeResult(errors.New("e1"))
	s.evalProbeResult(errors.New("e2"))
	if !s.evalProbeResult(errors.New("e3")) {
		t.Fatal("want exit at count=3, got no-exit")
	}
}

// TestProbeState_SuccessResetsCounter: a single recovery clears the counter.
// Cert-rotation transient errors must not accumulate across hours.
func TestProbeState_SuccessResetsCounter(t *testing.T) {
	var s probeState
	s.evalProbeResult(errors.New("e1"))
	s.evalProbeResult(errors.New("e2"))
	if s.evalProbeResult(nil) {
		t.Fatal("success must not trigger exit")
	}
	if s.consecutive != 0 {
		t.Fatalf("want counter reset to 0 after success, got %d", s.consecutive)
	}
	// Two more failures after reset must still not exit (the counter restarts).
	if s.evalProbeResult(errors.New("e3")) {
		t.Fatal("want no-exit at count=1 after reset")
	}
	if s.evalProbeResult(errors.New("e4")) {
		t.Fatal("want no-exit at count=2 after reset")
	}
}

// TestProbeState_Threshold: sanity that the constant is the expected value.
// Guards against a silent lowering of the threshold that would cause flapping
// in production on transient netlink blips.
func TestProbeState_Threshold(t *testing.T) {
	if probeFailureThreshold != 3 {
		t.Fatalf("want threshold=3 (ADR-000802: 30s interval × 3 = ~90s detection, matching k8s/Dapr sidecar defaults), got %d", probeFailureThreshold)
	}
}

// TestSelfProbeInterval_Cadence: the probe interval must stay within the
// industry-standard sidecar envelope (10s–60s). Locks in the decision
// recorded in ADR-000802 so a future change can't silently regress to the
// old 5 min cadence that left netns orphans undetected for ~15 minutes.
func TestSelfProbeInterval_Cadence(t *testing.T) {
	if selfProbeInterval < 10*time.Second {
		t.Fatalf("self-probe interval %s is too aggressive; will cause restart flapping on transient blips", selfProbeInterval)
	}
	if selfProbeInterval > 60*time.Second {
		t.Fatalf("self-probe interval %s leaves netns-orphan detection window > 3 min (ADR-000802)", selfProbeInterval)
	}
}
