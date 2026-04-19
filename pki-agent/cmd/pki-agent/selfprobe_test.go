package main

import (
	"errors"
	"testing"
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
// in production on normal cert-rotation retries.
func TestProbeState_Threshold(t *testing.T) {
	if probeFailureThreshold != 3 {
		t.Fatalf("want threshold=3 (ADR-000784 N=3, ~15 min at 5 min tick), got %d", probeFailureThreshold)
	}
}
