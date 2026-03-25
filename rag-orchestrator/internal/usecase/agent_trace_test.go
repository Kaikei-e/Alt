package usecase

import (
	"testing"
	"time"
)

func TestAgentTrace_AddStep(t *testing.T) {
	trace := NewAgentTrace("req-123")
	trace.AddStep("classify", "success", "comparison", 5*time.Millisecond)
	trace.AddStep("retrieve", "success", "10 chunks", 150*time.Millisecond)

	if len(trace.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(trace.Steps))
	}
	if trace.Steps[0].Name != "classify" {
		t.Errorf("expected step name 'classify', got %s", trace.Steps[0].Name)
	}
	if trace.Steps[1].DurationMs != 150 {
		t.Errorf("expected 150ms, got %d", trace.Steps[1].DurationMs)
	}
}

func TestAgentTrace_TotalDuration(t *testing.T) {
	trace := NewAgentTrace("req-456")
	trace.AddStep("classify", "success", "", 10*time.Millisecond)
	trace.AddStep("retrieve", "success", "", 200*time.Millisecond)
	trace.AddStep("generate", "success", "", 5000*time.Millisecond)

	total := trace.TotalDurationMs()
	if total != 5210 {
		t.Errorf("expected total 5210ms, got %d", total)
	}
}

func TestAgentTrace_EmptySteps(t *testing.T) {
	trace := NewAgentTrace("req-789")
	if trace.TotalDurationMs() != 0 {
		t.Errorf("expected 0ms for empty trace, got %d", trace.TotalDurationMs())
	}
	if len(trace.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(trace.Steps))
	}
}
