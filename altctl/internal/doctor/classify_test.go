package doctor

import (
	"strings"
	"testing"
)

// TestClassifyService_Paused and TestClassifyService_Dead are the M1
// regression tests: docker compose ps can report State "paused" (docker
// pause) and "dead" (failed to stop/remove cleanly), and the old default
// branch silently classified both as StateHealthy/not-a-problem. `altctl
// doctor` must flag them instead of staying silent.
func TestClassifyService_Paused(t *testing.T) {
	entry := &psEntry{Name: "alt-db-1", Service: "db", State: "paused"}

	label, severity, isProblem := classifyService(entry)

	if label != StatePaused {
		t.Errorf("label = %q, want %q", label, StatePaused)
	}
	if !isProblem {
		t.Error("expected a paused container to be a problem, got isProblem=false")
	}
	if severity == "" {
		t.Error("expected a non-empty severity for a paused container")
	}
}

func TestClassifyService_Dead(t *testing.T) {
	entry := &psEntry{Name: "alt-db-1", Service: "db", State: "dead"}

	label, severity, isProblem := classifyService(entry)

	if label != StateDead {
		t.Errorf("label = %q, want %q", label, StateDead)
	}
	if !isProblem {
		t.Error("expected a dead container to be a problem, got isProblem=false")
	}
	if severity != SeverityError {
		t.Errorf("severity = %q, want %q (dead is worse than paused)", severity, SeverityError)
	}
}

// TestClassifyService_UnknownState_DefaultsToProblem is the other half of
// M1: flip the default polarity from "assume healthy" to "assume a
// problem", mirroring internal/health/waiter.go's evaluateOne (not-ready
// unless explicitly recognized as good). "removing" is a real (if
// transient) docker compose ps State that isn't explicitly handled, used
// here as a stand-in for "whatever state this table hasn't been taught
// about yet."
func TestClassifyService_UnknownState_DefaultsToProblem(t *testing.T) {
	entry := &psEntry{Name: "alt-db-1", Service: "db", State: "removing"}

	label, _, isProblem := classifyService(entry)

	if label != StateUnknown {
		t.Errorf("label = %q, want %q", label, StateUnknown)
	}
	if !isProblem {
		t.Error("expected an unrecognized state to default to a problem, got isProblem=false")
	}
}

// --- Regression coverage: existing classifications must not change ---

func TestClassifyService_Missing(t *testing.T) {
	label, _, isProblem := classifyService(nil)
	if label != StateMissing || !isProblem {
		t.Errorf("got (%q, isProblem=%v), want (%q, true)", label, isProblem, StateMissing)
	}
}

func TestClassifyService_RunningNoHealthcheck_Healthy(t *testing.T) {
	entry := &psEntry{Name: "alt-fwd-1", Service: "log-forwarder", State: "running"}
	label, _, isProblem := classifyService(entry)
	if label != StateHealthy || isProblem {
		t.Errorf("got (%q, isProblem=%v), want (%q, false)", label, isProblem, StateHealthy)
	}
}

func TestClassifyService_RunningHealthy(t *testing.T) {
	entry := &psEntry{Name: "alt-db-1", Service: "db", State: "running", Health: "healthy"}
	label, _, isProblem := classifyService(entry)
	if label != StateHealthy || isProblem {
		t.Errorf("got (%q, isProblem=%v), want (%q, false)", label, isProblem, StateHealthy)
	}
}

func TestClassifyService_RunningUnhealthy(t *testing.T) {
	entry := &psEntry{Name: "alt-db-1", Service: "db", State: "running", Health: "unhealthy"}
	label, severity, isProblem := classifyService(entry)
	if label != StateUnhealthy || !isProblem || severity != SeverityError {
		t.Errorf("got (%q, %q, isProblem=%v), want (%q, %q, true)", label, severity, isProblem, StateUnhealthy, SeverityError)
	}
}

func TestClassifyService_Restarting(t *testing.T) {
	entry := &psEntry{Name: "alt-db-1", Service: "db", State: "restarting"}
	label, _, isProblem := classifyService(entry)
	if label != StateRestarting || !isProblem {
		t.Errorf("got (%q, isProblem=%v), want (%q, true)", label, isProblem, StateRestarting)
	}
}

func TestClassifyService_ExitedZero_OneShotHealthy(t *testing.T) {
	entry := &psEntry{Name: "alt-migrator-1", Service: "migrator", State: "exited", ExitCode: 0}
	label, _, isProblem := classifyService(entry)
	if label != StateHealthy || isProblem {
		t.Errorf("got (%q, isProblem=%v), want (%q, false) -- clean one-shot exit must not be a problem", label, isProblem, StateHealthy)
	}
}

func TestClassifyService_ExitedNonZero(t *testing.T) {
	entry := &psEntry{Name: "alt-migrator-1", Service: "migrator", State: "exited", ExitCode: 1}
	label, _, isProblem := classifyService(entry)
	if label != StateExitedNonZero || !isProblem {
		t.Errorf("got (%q, isProblem=%v), want (%q, true)", label, isProblem, StateExitedNonZero)
	}
}

func TestClassifyService_Created_Starting(t *testing.T) {
	entry := &psEntry{Name: "alt-db-1", Service: "db", State: "created"}
	label, _, isProblem := classifyService(entry)
	if label != StateStarting || !isProblem {
		t.Errorf("got (%q, isProblem=%v), want (%q, true)", label, isProblem, StateStarting)
	}
}

// --- Message/prescription plumbing for the new states ---

func TestBuildMessage_PausedAndDead_DistinctText(t *testing.T) {
	pausedMsg := buildMessage("db", StatePaused, "", "")
	deadMsg := buildMessage("db", StateDead, "", "")

	if pausedMsg == deadMsg {
		t.Fatal("paused and dead should render distinct messages")
	}
	if !strings.Contains(pausedMsg, "paused") {
		t.Errorf("paused message = %q, want it to mention 'paused'", pausedMsg)
	}
	if !strings.Contains(deadMsg, "dead") {
		t.Errorf("dead message = %q, want it to mention 'dead'", deadMsg)
	}
}

func TestPrescriptionFor_PausedAndDead_NotNil(t *testing.T) {
	if p := prescriptionFor(StatePaused, "db"); len(p) == 0 {
		t.Error("expected a non-empty prescription for a paused service")
	}
	if p := prescriptionFor(StateDead, "db"); len(p) == 0 {
		t.Error("expected a non-empty prescription for a dead service")
	}
	if p := prescriptionFor(StateUnknown, "db"); len(p) == 0 {
		t.Error("expected a non-empty prescription for an unknown-state service")
	}
}
