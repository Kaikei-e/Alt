package otel

import (
	"testing"
)

func TestNewKnowledgeHomeMetrics_AllFieldsInitialized(t *testing.T) {
	m, err := NewKnowledgeHomeMetrics()
	if err != nil {
		t.Fatalf("NewKnowledgeHomeMetrics() returned error: %v", err)
	}
	if m == nil {
		t.Fatal("NewKnowledgeHomeMetrics() returned nil")
	}

	// Existing projector metrics
	if m.ProjectorEventsProcessed == nil {
		t.Error("ProjectorEventsProcessed is nil")
	}
	if m.ProjectorLagSeconds == nil {
		t.Error("ProjectorLagSeconds is nil")
	}
	if m.ProjectorBatchDurationMs == nil {
		t.Error("ProjectorBatchDurationMs is nil")
	}
	if m.ProjectorErrors == nil {
		t.Error("ProjectorErrors is nil")
	}

	// Existing handler metrics
	if m.PageServed == nil {
		t.Error("PageServed is nil")
	}
	if m.PageDegraded == nil {
		t.Error("PageDegraded is nil")
	}

	// Existing tracking metrics
	if m.ItemsExposed == nil {
		t.Error("ItemsExposed is nil")
	}
	if m.ItemsOpened == nil {
		t.Error("ItemsOpened is nil")
	}
	if m.ItemsDismissed == nil {
		t.Error("ItemsDismissed is nil")
	}

	// Existing backfill metrics
	if m.BackfillEventsGenerated == nil {
		t.Error("BackfillEventsGenerated is nil")
	}

	// Phase 5: SLI-A availability
	if m.RequestsTotal == nil {
		t.Error("RequestsTotal is nil")
	}
	if m.RequestDurationSeconds == nil {
		t.Error("RequestDurationSeconds is nil")
	}
	if m.DegradedResponsesTotal == nil {
		t.Error("DegradedResponsesTotal is nil")
	}
	if m.ProjectionAgeSeconds == nil {
		t.Error("ProjectionAgeSeconds is nil")
	}

	// Phase 5: SLI-C durability
	if m.TrackingReceivedTotal == nil {
		t.Error("TrackingReceivedTotal is nil")
	}
	if m.TrackingPersistedTotal == nil {
		t.Error("TrackingPersistedTotal is nil")
	}
	if m.TrackingFailedTotal == nil {
		t.Error("TrackingFailedTotal is nil")
	}

	// Phase 5: SLI-D stream
	if m.StreamConnectionsTotal == nil {
		t.Error("StreamConnectionsTotal is nil")
	}
	if m.StreamDisconnectsTotal == nil {
		t.Error("StreamDisconnectsTotal is nil")
	}
	if m.StreamReconnectsTotal == nil {
		t.Error("StreamReconnectsTotal is nil")
	}
	if m.StreamDeliveriesTotal == nil {
		t.Error("StreamDeliveriesTotal is nil")
	}
	if m.StreamUpdateLagSeconds == nil {
		t.Error("StreamUpdateLagSeconds is nil")
	}

	// Phase 5: SLI-E correctness
	if m.EmptyResponsesTotal == nil {
		t.Error("EmptyResponsesTotal is nil")
	}
	if m.MalformedWhyTotal == nil {
		t.Error("MalformedWhyTotal is nil")
	}
	if m.OrphanItemsTotal == nil {
		t.Error("OrphanItemsTotal is nil")
	}
	if m.SupersedeMismatchTotal == nil {
		t.Error("SupersedeMismatchTotal is nil")
	}

	// Phase 5: reproject
	if m.ReprojectEventsTotal == nil {
		t.Error("ReprojectEventsTotal is nil")
	}

	// Sovereign metrics
	if m.SovereignMutationApplied == nil {
		t.Error("SovereignMutationApplied is nil")
	}
	if m.SovereignMutationError == nil {
		t.Error("SovereignMutationError is nil")
	}
	if m.SovereignMutationDuration == nil {
		t.Error("SovereignMutationDuration is nil")
	}
	if m.SovereignReconciliationRun == nil {
		t.Error("SovereignReconciliationRun is nil")
	}
	if m.SovereignReconciliationMismatch == nil {
		t.Error("SovereignReconciliationMismatch is nil")
	}
	if m.SovereignReconciliationDuration == nil {
		t.Error("SovereignReconciliationDuration is nil")
	}
	if m.SovereignIdempotencyDuplicate == nil {
		t.Error("SovereignIdempotencyDuplicate is nil")
	}
	if m.SovereignCutoverReady == nil {
		t.Error("SovereignCutoverReady is nil")
	}
}
