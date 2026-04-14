package otel

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

// MetricsSnapshot provides read access to accumulated metric values for the admin API.
// Updated in parallel with OTel instruments via Record* methods on KnowledgeHomeMetrics.
type MetricsSnapshot struct {
	// Projector
	projectorEventsProcessed atomic.Int64
	projectorLagSeconds      atomic.Uint64 // float64 bits
	projectorErrors          atomic.Int64
	projectorBatchDurations  *durationTracker

	// Handler
	pagesServed   atomic.Int64
	pagesDegraded atomic.Int64

	// Tracking
	itemsExposed   atomic.Int64
	itemsOpened    atomic.Int64
	itemsDismissed atomic.Int64

	// SLI-A: availability
	requestsTotal atomic.Int64
	degradedTotal atomic.Int64

	// SLI-C: durability
	trackingReceived  atomic.Int64
	trackingPersisted atomic.Int64
	trackingFailed    atomic.Int64

	// SLI-D: stream
	streamConnections atomic.Int64
	streamDisconnects atomic.Int64
	streamReconnects  atomic.Int64
	streamDeliveries  atomic.Int64

	// SLI-E: correctness
	emptyResponses    atomic.Int64
	malformedWhy      atomic.Int64
	orphanItems       atomic.Int64
	supersedeMismatch atomic.Int64

	// Sovereign
	sovereignApplied   atomic.Int64
	sovereignErrors    atomic.Int64
	sovereignDurations *durationTracker

	// Recall
	recallSignals            atomic.Int64
	recallSignalErrors       atomic.Int64
	recallCandidates         atomic.Int64
	recallCandidatesEmpty    atomic.Int64
	recallUsersProcessed     atomic.Int64
	recallProjectorDurations *durationTracker
}

// NewMetricsSnapshot creates a new snapshot tracker.
func NewMetricsSnapshot() *MetricsSnapshot {
	return &MetricsSnapshot{
		projectorBatchDurations:  newDurationTracker(1000),
		sovereignDurations:       newDurationTracker(1000),
		recallProjectorDurations: newDurationTracker(1000),
	}
}

// --- Projector ---

func (s *MetricsSnapshot) RecordProjectorEvent() { s.projectorEventsProcessed.Add(1) }
func (s *MetricsSnapshot) RecordProjectorError() { s.projectorErrors.Add(1) }
func (s *MetricsSnapshot) RecordProjectorLag(v float64) {
	s.projectorLagSeconds.Store(math.Float64bits(v))
}
func (s *MetricsSnapshot) RecordProjectorBatch(ms float64) { s.projectorBatchDurations.Record(ms) }

func (s *MetricsSnapshot) ProjectorEventsProcessed() int64 { return s.projectorEventsProcessed.Load() }
func (s *MetricsSnapshot) ProjectorErrors() int64          { return s.projectorErrors.Load() }
func (s *MetricsSnapshot) ProjectorLagSeconds() float64 {
	return math.Float64frombits(s.projectorLagSeconds.Load())
}
func (s *MetricsSnapshot) ProjectorBatchP50() float64 {
	return s.projectorBatchDurations.Percentile(50)
}
func (s *MetricsSnapshot) ProjectorBatchP95() float64 {
	return s.projectorBatchDurations.Percentile(95)
}
func (s *MetricsSnapshot) ProjectorBatchP99() float64 {
	return s.projectorBatchDurations.Percentile(99)
}

// --- Handler ---

func (s *MetricsSnapshot) RecordPageServed()   { s.pagesServed.Add(1) }
func (s *MetricsSnapshot) RecordPageDegraded() { s.pagesDegraded.Add(1) }

func (s *MetricsSnapshot) PagesServed() int64   { return s.pagesServed.Load() }
func (s *MetricsSnapshot) PagesDegraded() int64 { return s.pagesDegraded.Load() }

// --- Tracking ---

func (s *MetricsSnapshot) RecordItemExposed()   { s.itemsExposed.Add(1) }
func (s *MetricsSnapshot) RecordItemOpened()    { s.itemsOpened.Add(1) }
func (s *MetricsSnapshot) RecordItemDismissed() { s.itemsDismissed.Add(1) }

func (s *MetricsSnapshot) ItemsExposed() int64   { return s.itemsExposed.Load() }
func (s *MetricsSnapshot) ItemsOpened() int64    { return s.itemsOpened.Load() }
func (s *MetricsSnapshot) ItemsDismissed() int64 { return s.itemsDismissed.Load() }

// --- SLI-A ---

func (s *MetricsSnapshot) RecordRequest()          { s.requestsTotal.Add(1) }
func (s *MetricsSnapshot) RecordDegradedResponse() { s.degradedTotal.Add(1) }

func (s *MetricsSnapshot) RequestsTotal() int64 { return s.requestsTotal.Load() }
func (s *MetricsSnapshot) DegradedTotal() int64 { return s.degradedTotal.Load() }

// --- SLI-C ---

func (s *MetricsSnapshot) RecordTrackingReceived()  { s.trackingReceived.Add(1) }
func (s *MetricsSnapshot) RecordTrackingPersisted() { s.trackingPersisted.Add(1) }
func (s *MetricsSnapshot) RecordTrackingFailed()    { s.trackingFailed.Add(1) }

func (s *MetricsSnapshot) TrackingReceived() int64  { return s.trackingReceived.Load() }
func (s *MetricsSnapshot) TrackingPersisted() int64 { return s.trackingPersisted.Load() }
func (s *MetricsSnapshot) TrackingFailed() int64    { return s.trackingFailed.Load() }

// --- SLI-D ---

func (s *MetricsSnapshot) RecordStreamConnection() { s.streamConnections.Add(1) }
func (s *MetricsSnapshot) RecordStreamDisconnect() { s.streamDisconnects.Add(1) }
func (s *MetricsSnapshot) RecordStreamReconnect()  { s.streamReconnects.Add(1) }
func (s *MetricsSnapshot) RecordStreamDelivery()   { s.streamDeliveries.Add(1) }

func (s *MetricsSnapshot) StreamConnections() int64 { return s.streamConnections.Load() }
func (s *MetricsSnapshot) StreamDisconnects() int64 { return s.streamDisconnects.Load() }
func (s *MetricsSnapshot) StreamReconnects() int64  { return s.streamReconnects.Load() }
func (s *MetricsSnapshot) StreamDeliveries() int64  { return s.streamDeliveries.Load() }

// --- SLI-E ---

func (s *MetricsSnapshot) RecordEmptyResponse()     { s.emptyResponses.Add(1) }
func (s *MetricsSnapshot) RecordMalformedWhy()      { s.malformedWhy.Add(1) }
func (s *MetricsSnapshot) RecordOrphanItem()        { s.orphanItems.Add(1) }
func (s *MetricsSnapshot) RecordSupersedeMismatch() { s.supersedeMismatch.Add(1) }

func (s *MetricsSnapshot) EmptyResponses() int64    { return s.emptyResponses.Load() }
func (s *MetricsSnapshot) MalformedWhy() int64      { return s.malformedWhy.Load() }
func (s *MetricsSnapshot) OrphanItems() int64       { return s.orphanItems.Load() }
func (s *MetricsSnapshot) SupersedeMismatch() int64 { return s.supersedeMismatch.Load() }

// --- Sovereign ---

func (s *MetricsSnapshot) RecordSovereignApplied()            { s.sovereignApplied.Add(1) }
func (s *MetricsSnapshot) RecordSovereignError()              { s.sovereignErrors.Add(1) }
func (s *MetricsSnapshot) RecordSovereignDuration(ms float64) { s.sovereignDurations.Record(ms) }

func (s *MetricsSnapshot) SovereignApplied() int64       { return s.sovereignApplied.Load() }
func (s *MetricsSnapshot) SovereignErrors() int64        { return s.sovereignErrors.Load() }
func (s *MetricsSnapshot) SovereignDurationP50() float64 { return s.sovereignDurations.Percentile(50) }
func (s *MetricsSnapshot) SovereignDurationP95() float64 { return s.sovereignDurations.Percentile(95) }

// --- Recall ---

func (s *MetricsSnapshot) RecordRecallSignal()         { s.recallSignals.Add(1) }
func (s *MetricsSnapshot) RecordRecallSignalError()    { s.recallSignalErrors.Add(1) }
func (s *MetricsSnapshot) RecordRecallCandidate()      { s.recallCandidates.Add(1) }
func (s *MetricsSnapshot) RecordRecallCandidateEmpty() { s.recallCandidatesEmpty.Add(1) }
func (s *MetricsSnapshot) RecordRecallUserProcessed()  { s.recallUsersProcessed.Add(1) }
func (s *MetricsSnapshot) RecordRecallProjectorDuration(ms float64) {
	s.recallProjectorDurations.Record(ms)
}

func (s *MetricsSnapshot) RecallSignals() int64         { return s.recallSignals.Load() }
func (s *MetricsSnapshot) RecallSignalErrors() int64    { return s.recallSignalErrors.Load() }
func (s *MetricsSnapshot) RecallCandidates() int64      { return s.recallCandidates.Load() }
func (s *MetricsSnapshot) RecallCandidatesEmpty() int64 { return s.recallCandidatesEmpty.Load() }
func (s *MetricsSnapshot) RecallUsersProcessed() int64  { return s.recallUsersProcessed.Load() }
func (s *MetricsSnapshot) RecallProjectorDurationP50() float64 {
	return s.recallProjectorDurations.Percentile(50)
}
func (s *MetricsSnapshot) RecallProjectorDurationP95() float64 {
	return s.recallProjectorDurations.Percentile(95)
}

// durationTracker is a ring-buffer based percentile tracker for histogram values.
type durationTracker struct {
	mu      sync.Mutex
	samples []float64
	pos     int
	maxSize int
	filled  bool
}

func newDurationTracker(maxSize int) *durationTracker {
	return &durationTracker{
		samples: make([]float64, maxSize),
		maxSize: maxSize,
	}
}

func (t *durationTracker) Record(v float64) {
	t.mu.Lock()
	t.samples[t.pos] = v
	t.pos++
	if t.pos >= t.maxSize {
		t.pos = 0
		t.filled = true
	}
	t.mu.Unlock()
}

func (t *durationTracker) Percentile(pct float64) float64 {
	t.mu.Lock()
	n := t.pos
	if t.filled {
		n = t.maxSize
	}
	if n == 0 {
		t.mu.Unlock()
		return 0
	}
	sorted := make([]float64, n)
	if t.filled {
		copy(sorted, t.samples)
	} else {
		copy(sorted, t.samples[:n])
	}
	t.mu.Unlock()

	sort.Float64s(sorted)
	idx := int(float64(len(sorted)-1) * pct / 100)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
