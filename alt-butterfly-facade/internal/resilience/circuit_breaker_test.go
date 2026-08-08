package resilience

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testEndpoint      = "/alt.feeds.v2.FeedService/GetUnreadFeeds"
	otherTestEndpoint = "/alt.feeds.v2.FeedService/GetUnreadCount"
)

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	})

	assert.NotNil(t, cb)
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.Allow(testEndpoint))
}

func TestCircuitBreaker_StaysClosedOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	for i := 0; i < 10; i++ {
		assert.True(t, cb.Allow(testEndpoint))
		cb.RecordSuccess(testEndpoint)
	}

	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	})

	// Record failures up to threshold
	for i := 0; i < 5; i++ {
		assert.True(t, cb.Allow(testEndpoint))
		cb.RecordFailure(testEndpoint)
	}

	// Circuit should be open now
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow(testEndpoint))
}

func TestCircuitBreaker_RejectsWhenOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      1 * time.Hour, // Long timeout to ensure it stays open
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}

	// All subsequent requests should be rejected
	for i := 0; i < 10; i++ {
		assert.False(t, cb.Allow(testEndpoint))
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}

	assert.Equal(t, StateOpen, cb.State())

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open and allow one request
	assert.True(t, cb.Allow(testEndpoint))
	assert.Equal(t, StateHalfOpen, cb.State())
}

func TestCircuitBreaker_ClosesAfterSuccessInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Record successes in half-open state
	cb.Allow(testEndpoint)
	cb.RecordSuccess(testEndpoint)
	cb.Allow(testEndpoint)
	cb.RecordSuccess(testEndpoint)

	// Should be closed now
	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.Allow(testEndpoint))
}

func TestCircuitBreaker_ReopensOnFailureInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Allow one request
	cb.Allow(testEndpoint)
	assert.Equal(t, StateHalfOpen, cb.State())

	// Record a failure
	cb.RecordFailure(testEndpoint)

	// Should be back to open
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow(testEndpoint))
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	})

	// Record some failures (but not enough to trip)
	for i := 0; i < 4; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}

	// Record a success
	cb.Allow(testEndpoint)
	cb.RecordSuccess(testEndpoint)

	// Failure count should be reset, so more failures needed to trip
	for i := 0; i < 4; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}

	// Should still be closed
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_Execute_Success(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	result, err := Execute(cb, testEndpoint, func() (string, error) {
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestCircuitBreaker_Execute_Failure(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	expectedErr := errors.New("operation failed")

	result, err := Execute(cb, testEndpoint, func() (string, error) {
		return "", expectedErr
	})

	assert.Equal(t, expectedErr, err)
	assert.Equal(t, "", result)
}

func TestCircuitBreaker_Execute_CircuitOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		OpenTimeout:      1 * time.Hour,
	})

	// Trip the circuit
	for i := 0; i < 2; i++ {
		Execute(cb, testEndpoint, func() (string, error) {
			return "", errors.New("fail")
		})
	}

	// Next execution should fail immediately
	_, err := Execute(cb, testEndpoint, func() (string, error) {
		return "should not execute", nil
	})

	assert.Error(t, err)
	assert.Equal(t, ErrCircuitOpen, err)
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 100,
		SuccessThreshold: 10,
		OpenTimeout:      30 * time.Second,
	})

	done := make(chan bool)
	var successesAllowed, failuresAllowed int64

	// Concurrent successes
	go func() {
		for i := 0; i < 50; i++ {
			if cb.Allow(testEndpoint) {
				atomic.AddInt64(&successesAllowed, 1)
				cb.RecordSuccess(testEndpoint)
			}
		}
		done <- true
	}()

	// Concurrent failures
	go func() {
		for i := 0; i < 50; i++ {
			if cb.Allow(testEndpoint) {
				atomic.AddInt64(&failuresAllowed, 1)
				cb.RecordFailure(testEndpoint)
			}
		}
		done <- true
	}()

	// Concurrent state checks
	go func() {
		for i := 0; i < 50; i++ {
			cb.State()
			cb.Allow(testEndpoint)
		}
		done <- true
	}()

	<-done
	<-done
	<-done

	// FailureThreshold is 100 and at most 50 failures can ever be recorded,
	// so the circuit never opens and every Allow() call above returns true.
	// Stats() must therefore reflect exactly the calls this test allowed
	// through — a lost update here (e.g. a non-atomic counter, or a missing
	// lock in Stats()) would indicate a real data race, which this
	// invariant check surfaces under `go test -race` where the old
	// "didn't panic" assertion could not.
	stats := cb.Stats()
	assert.Equal(t, successesAllowed, stats.TotalSuccesses,
		"recorded successes must match the calls this test allowed through")
	assert.Equal(t, failuresAllowed, stats.TotalFailures,
		"recorded failures must match the calls this test allowed through")
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	// Record some activity
	for i := 0; i < 5; i++ {
		cb.Allow(testEndpoint)
		cb.RecordSuccess(testEndpoint)
	}
	for i := 0; i < 3; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}

	stats := cb.Stats()

	assert.Equal(t, int64(5), stats.TotalSuccesses)
	assert.Equal(t, int64(3), stats.TotalFailures)
	assert.Equal(t, StateClosed, stats.State)
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	assert.Equal(t, 5, config.FailureThreshold)
	assert.Equal(t, 2, config.SuccessThreshold)
	assert.Equal(t, 30*time.Second, config.OpenTimeout)
}

func TestCircuitBreakerState_String(t *testing.T) {
	assert.Equal(t, "CLOSED", StateClosed.String())
	assert.Equal(t, "OPEN", StateOpen.String())
	assert.Equal(t, "HALF_OPEN", StateHalfOpen.String())
}

func TestCircuitBreaker_HalfOpenLimitsRequests(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 3,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 2; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// First request allowed
	assert.True(t, cb.Allow(testEndpoint))
	assert.Equal(t, StateHalfOpen, cb.State())

	// Subsequent requests should be limited until the first completes
	assert.False(t, cb.Allow(testEndpoint))
	assert.False(t, cb.Allow(testEndpoint))

	// Completing the in-flight trial (success) frees up the slot again
	cb.RecordSuccess(testEndpoint)
	assert.True(t, cb.Allow(testEndpoint))
}

func TestCircuitBreaker_HalfOpenSlotFreedOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 2; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// First trial allowed, second rejected while trial is in flight
	assert.True(t, cb.Allow(testEndpoint))
	assert.False(t, cb.Allow(testEndpoint))

	// Trial fails -> circuit re-opens and the in-flight slot is released
	cb.RecordFailure(testEndpoint)
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow(testEndpoint))
}

// TestCircuitBreaker_HalfOpenTrialWithoutOutcomeSelfHeals covers the wedge:
// the request that takes the single half-open trial permit returns early
// (BFF cache hit) without ever calling RecordSuccess or RecordFailure. The
// permit must not stay outstanding forever — the half-open branch has to be
// time-aware so the class re-tests the backend instead of rejecting every
// later request until the process restarts.
func TestCircuitBreaker_HalfOpenTrialWithoutOutcomeSelfHeals(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 1,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}
	require.Equal(t, StateOpen, cb.State())

	// Wait for half-open, then take the trial permit and record nothing
	time.Sleep(60 * time.Millisecond)
	require.True(t, cb.Allow(testEndpoint))

	// While the trial is still fresh the slot stays reserved
	assert.False(t, cb.Allow(testEndpoint))

	// Once the trial has outlived its bound the abandoned permit is reclaimed
	time.Sleep(60 * time.Millisecond)
	assert.True(t, cb.Allow(testEndpoint), "an unresolved trial permit must be reclaimed, not wedge the circuit shut")
}

// TestCircuitBreaker_ReleaseFreesHalfOpenTrialSlot is the `defer
// permit.Release()` shape callers are expected to use: the trial slot comes
// back immediately when the holder finishes without recording an outcome,
// rather than waiting out the reclaim timeout.
func TestCircuitBreaker_ReleaseFreesHalfOpenTrialSlot(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 1,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	permit, allowed := cb.Acquire(testEndpoint)
	require.True(t, allowed)
	assert.Equal(t, 1, cb.Stats().HalfOpenInFlight)

	// Caller returns early without recording an outcome
	permit.Release()
	assert.Equal(t, 0, cb.Stats().HalfOpenInFlight)

	next, allowed := cb.Acquire(testEndpoint)
	assert.True(t, allowed, "released trial slot must be available to the next request")

	// Releasing a superseded or already-resolved permit must not double-free
	permit.Release()
	next.Release()
	next.Release()
	assert.Equal(t, 0, cb.Stats().HalfOpenInFlight)
}

// TestCircuitBreaker_ReleaseAfterRecordedOutcomeIsNoop guards the boundary
// between the two ways a trial resolves: RecordSuccess/RecordFailure already
// freed the slot, so the deferred Release must not free it a second time and
// let two trials run at once.
func TestCircuitBreaker_ReleaseAfterRecordedOutcomeIsNoop(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 3,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit, then wait for half-open
	for i := 0; i < 2; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}
	time.Sleep(60 * time.Millisecond)

	first, allowed := cb.Acquire(testEndpoint)
	require.True(t, allowed)
	cb.RecordSuccess(testEndpoint)

	second, allowed := cb.Acquire(testEndpoint)
	require.True(t, allowed)

	// The first permit is stale now; releasing it must not free the second slot
	first.Release()
	assert.Equal(t, 1, cb.Stats().HalfOpenInFlight)
	assert.False(t, cb.Allow(testEndpoint), "a stale Release must not admit a second concurrent trial")

	second.Release()
	assert.True(t, cb.Allow(testEndpoint))
}

// TestCircuitBreaker_AcquireFullTransitionCycle pins the normal
// closed -> open -> half-open -> closed path through the permit API, matching
// the transitions the Allow()-based tests above assert.
func TestCircuitBreaker_AcquireFullTransitionCycle(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Closed: every request admitted, permits carry no trial slot
	for i := 0; i < 2; i++ {
		permit, allowed := cb.Acquire(testEndpoint)
		require.True(t, allowed)
		cb.RecordSuccess(testEndpoint)
		permit.Release()
	}
	require.Equal(t, StateClosed, cb.State())
	assert.Equal(t, 0, cb.Stats().HalfOpenInFlight)

	// Closed -> open on consecutive failures
	for i := 0; i < 3; i++ {
		permit, allowed := cb.Acquire(testEndpoint)
		require.True(t, allowed)
		cb.RecordFailure(testEndpoint)
		permit.Release()
	}
	require.Equal(t, StateOpen, cb.State())
	_, allowed := cb.Acquire(testEndpoint)
	assert.False(t, allowed, "open circuit must reject requests")

	// Open -> half-open after OpenTimeout, one trial at a time
	time.Sleep(60 * time.Millisecond)
	trial, allowed := cb.Acquire(testEndpoint)
	require.True(t, allowed)
	require.Equal(t, StateHalfOpen, cb.State())
	_, allowed = cb.Acquire(testEndpoint)
	assert.False(t, allowed, "half-open must admit only one trial at a time")

	// Half-open -> closed once SuccessThreshold trials succeed
	cb.RecordSuccess(testEndpoint)
	trial.Release()
	assert.Equal(t, StateHalfOpen, cb.State())

	trial, allowed = cb.Acquire(testEndpoint)
	require.True(t, allowed)
	cb.RecordSuccess(testEndpoint)
	trial.Release()

	assert.Equal(t, StateClosed, cb.State())
	assert.Equal(t, 0, cb.Stats().HalfOpenInFlight)
}

// TestCircuitBreaker_HalfOpenPermitAccountingUnderConcurrency hammers the
// half-open trial slot from many goroutines, mixing trials that record an
// outcome with trials that return early and only release. The ledger must
// never leak a slot (which wedges the class) and never hand out two at once.
func TestCircuitBreaker_HalfOpenPermitAccountingUnderConcurrency(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		// High enough that the storm never closes the circuit, so every
		// iteration keeps exercising the half-open permit ledger. OpenTimeout
		// is long relative to the storm so no trial is ever held long enough
		// to hit the abandoned-permit reclaim.
		SuccessThreshold: 1_000_000,
		OpenTimeout:      300 * time.Millisecond,
	})

	// Trip the circuit, then settle into half-open with the slot free
	for i := 0; i < 2; i++ {
		cb.Allow(testEndpoint)
		cb.RecordFailure(testEndpoint)
	}
	require.Equal(t, StateOpen, cb.State())
	time.Sleep(320 * time.Millisecond)
	warmup, allowed := cb.Acquire(testEndpoint)
	require.True(t, allowed)
	warmup.Release()
	require.Equal(t, StateHalfOpen, cb.State())

	const (
		goroutines = 32
		iterations = 50
	)
	var granted, ledgerViolations int64

	// sampleLedger records any moment the breaker's own trial ledger leaves
	// [0, 1]: above 1 means two trials were admitted at once, below 0 means a
	// permit was freed twice.
	sampleLedger := func() {
		if inFlight := cb.Stats().HalfOpenInFlight; inFlight < 0 || inFlight > 1 {
			atomic.AddInt64(&ledgerViolations, 1)
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				permit, allowed := cb.Acquire(testEndpoint)
				if !allowed {
					sampleLedger()
					continue
				}
				atomic.AddInt64(&granted, 1)
				sampleLedger()

				// Half the trials reach the dependency and record an outcome;
				// the other half return early (cache hit) and only release.
				if j%2 == 0 {
					cb.RecordSuccess(testEndpoint)
					sampleLedger()
				}
				permit.Release()
				sampleLedger()
			}
		}()
	}
	wg.Wait()

	assert.Positive(t, granted, "the half-open slot must keep being handed out")
	assert.Zero(t, ledgerViolations, "at most one trial may be outstanding, and none may be freed twice")
	assert.Equal(t, 0, cb.Stats().HalfOpenInFlight, "every granted trial permit must be handed back")
	assert.True(t, cb.Allow(testEndpoint), "a sound permit ledger must still admit the next trial")
}

// transitionRecorder is the observer under test: it captures what an operator
// would have been told, so a test can assert both the content of each report
// and that there is exactly one report per state change.
type transitionRecorder struct {
	mu   sync.Mutex
	seen []StateTransition
}

func (r *transitionRecorder) observe(tr StateTransition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, tr)
}

func (r *transitionRecorder) snapshot() []StateTransition {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]StateTransition, len(r.seen))
	copy(out, r.seen)
	return out
}

func (r *transitionRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.seen)
}

func transitionEdges(trs []StateTransition) [][2]CircuitState {
	out := make([][2]CircuitState, 0, len(trs))
	for _, tr := range trs {
		out = append(out, [2]CircuitState{tr.From, tr.To})
	}
	return out
}

// TestCircuitBreaker_ReportsEveryStateTransition pins the whole observable
// lifecycle: an operator must be able to see the circuit trip, re-probe,
// trip again, and recover, without polling anything.
func TestCircuitBreaker_ReportsEveryStateTransition(t *testing.T) {
	rec := &transitionRecorder{}
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
		Class:            ClassUnreadProjection,
		OnTransition:     rec.observe,
	})

	// CLOSED -> OPEN
	for i := 0; i < 3; i++ {
		require.True(t, cb.Allow(testEndpoint))
		cb.RecordFailure(testEndpoint)
	}
	require.Equal(t, StateOpen, cb.State())

	// OPEN -> HALF_OPEN -> OPEN
	time.Sleep(60 * time.Millisecond)
	require.True(t, cb.Allow(testEndpoint))
	cb.RecordFailure(testEndpoint)

	// OPEN -> HALF_OPEN -> CLOSED
	time.Sleep(60 * time.Millisecond)
	require.True(t, cb.Allow(testEndpoint))
	cb.RecordSuccess(testEndpoint)
	require.True(t, cb.Allow(testEndpoint))
	cb.RecordSuccess(testEndpoint)
	require.Equal(t, StateClosed, cb.State())

	seen := rec.snapshot()
	assert.Equal(t, [][2]CircuitState{
		{StateClosed, StateOpen},
		{StateOpen, StateHalfOpen},
		{StateHalfOpen, StateOpen},
		{StateOpen, StateHalfOpen},
		{StateHalfOpen, StateClosed},
	}, transitionEdges(seen))

	tripped := seen[0]
	assert.Equal(t, ClassUnreadProjection, tripped.Class, "the report must name the dependency class")
	assert.Equal(t, testEndpoint, tripped.Endpoint, "the report must name the endpoint that tripped it")
	assert.Equal(t, 3, tripped.ConsecFailures)
	assert.Equal(t, 3, tripped.FailureThreshold)
	assert.Equal(t, 2, tripped.SuccessThreshold)
	assert.Equal(t, 50*time.Millisecond, tripped.OpenTimeout)
	assert.Equal(t, uint64(1), tripped.Seq)

	recovered := seen[len(seen)-1]
	assert.Equal(t, 2, recovered.ConsecSuccesses)
	assert.Equal(t, uint64(5), recovered.Seq)
}

// TestCircuitBreaker_AggregatesRejectionsRatherThanReportingEachOne is the
// log-flood guard: an open breaker refusing thousands of requests must stay
// silent per request and account for them in the next transition report.
func TestCircuitBreaker_AggregatesRejectionsRatherThanReportingEachOne(t *testing.T) {
	rec := &transitionRecorder{}
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		OpenTimeout:      50 * time.Millisecond,
		Class:            ClassCriticalMutation,
		OnTransition:     rec.observe,
	})

	for i := 0; i < 2; i++ {
		require.True(t, cb.Allow(testEndpoint))
		cb.RecordFailure(testEndpoint)
	}
	require.Equal(t, 1, rec.count())

	const rejections = 500
	for i := 0; i < rejections; i++ {
		require.False(t, cb.Allow(otherTestEndpoint))
	}
	assert.Equal(t, 1, rec.count(), "an open breaker must not report once per rejected request")
	assert.Equal(t, int64(rejections), cb.Stats().TotalRejections)

	time.Sleep(60 * time.Millisecond)
	require.True(t, cb.Allow(testEndpoint))

	seen := rec.snapshot()
	require.Len(t, seen, 2)
	assert.Equal(t, int64(rejections), seen[1].RejectedSinceLastReport,
		"the transition out of OPEN must carry how many requests the open breaker refused")

	cb.RecordSuccess(testEndpoint)
	seen = rec.snapshot()
	require.Len(t, seen, 3)
	assert.Zero(t, seen[2].RejectedSinceLastReport, "the per-report tally resets once reported")
	assert.Equal(t, int64(rejections), cb.Stats().TotalRejections,
		"the cumulative tally survives for /v1/bff/stats")
}

// TestCircuitBreaker_ObserverRunsOutsideTheBreakerLock reads the breaker back
// from inside the observer. sync.RWMutex is not reentrant, so this wedges
// forever if the report is delivered while the write lock is still held —
// which under load would put a logger write on every request's critical path.
func TestCircuitBreaker_ObserverRunsOutsideTheBreakerLock(t *testing.T) {
	var cb *CircuitBreaker
	observed := make(chan CircuitBreakerStats, 1)
	cb = NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		OpenTimeout:      time.Hour,
		OnTransition: func(StateTransition) {
			observed <- cb.Stats()
		},
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		cb.RecordFailure(testEndpoint)
	}()

	select {
	case stats := <-observed:
		assert.Equal(t, StateOpen, stats.State)
	case <-time.After(5 * time.Second):
		t.Fatal("observer was invoked while the breaker lock was held")
	}
	<-done
}

// TestCircuitBreaker_ReportsOneOpenTransitionUnderConcurrentFailures: many
// goroutines cross the failure threshold at once, and that is still one
// transition — not one per goroutine that saw the counter pass the line.
func TestCircuitBreaker_ReportsOneOpenTransitionUnderConcurrentFailures(t *testing.T) {
	rec := &transitionRecorder{}
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      time.Hour,
		Class:            ClassNonCritical,
		OnTransition:     rec.observe,
	})

	const goroutines, iterations = 32, 40
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				permit, allowed := cb.Acquire(testEndpoint)
				if !allowed {
					continue
				}
				cb.RecordFailure(testEndpoint)
				permit.Release()
			}
		}()
	}
	wg.Wait()

	seen := rec.snapshot()
	require.Len(t, seen, 1, "concurrent failures past the threshold are one transition")
	assert.Equal(t, [][2]CircuitState{{StateClosed, StateOpen}}, transitionEdges(seen))
	assert.GreaterOrEqual(t, seen[0].ConsecFailures, 5)
	assert.Equal(t, ClassNonCritical, seen[0].Class)
}

// TestCircuitBreaker_TransitionReportsAreExactlyOnceUnderConcurrency storms the
// breaker through repeated trip/probe/recover cycles from many goroutines. The
// pin is the sequence: reports are delivered outside the lock and so may arrive
// out of order, but ordering them by Seq must yield 1..N with no gap (a lost
// transition) and no repeat (a double report), chaining state end to end.
func TestCircuitBreaker_TransitionReportsAreExactlyOnceUnderConcurrency(t *testing.T) {
	rec := &transitionRecorder{}
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		OpenTimeout:      time.Millisecond,
		Class:            ClassExternalContent,
		OnTransition:     rec.observe,
	})

	const goroutines, iterations = 16, 200
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				permit, allowed := cb.Acquire(testEndpoint)
				if !allowed {
					continue
				}
				if (seed+j)%3 == 0 {
					cb.RecordSuccess(testEndpoint)
				} else {
					cb.RecordFailure(testEndpoint)
				}
				permit.Release()
			}
		}(i)
	}
	wg.Wait()

	seen := rec.snapshot()
	require.NotEmpty(t, seen, "a storm this size must trip the circuit at least once")

	sort.Slice(seen, func(i, j int) bool { return seen[i].Seq < seen[j].Seq })
	prev := StateClosed
	for i, tr := range seen {
		require.Equal(t, uint64(i+1), tr.Seq, "transition sequence must be gap-free and unique")
		require.Equal(t, prev, tr.From, "each transition must start where the previous one ended")
		require.NotEqual(t, tr.From, tr.To, "a no-op is not a transition")
		prev = tr.To
	}
	// Stats reports the stored state; State() layers the lazy "open long enough
	// to be half-open" view over it, which no transition has happened for yet.
	assert.Equal(t, prev, cb.Stats().State, "the last report must describe the state the breaker is actually in")
}

// TestCircuitBreaker_NoObserverConfiguredIsSafe: the breaker is usable without
// an observer (tests, and the feature-flagged-off path).
func TestCircuitBreaker_NoObserverConfiguredIsSafe(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		OpenTimeout:      time.Millisecond,
	})

	require.True(t, cb.Allow(testEndpoint))
	cb.RecordFailure(testEndpoint)
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow(testEndpoint))
	assert.Equal(t, int64(1), cb.Stats().TotalRejections)
}
