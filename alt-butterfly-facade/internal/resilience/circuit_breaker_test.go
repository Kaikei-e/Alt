package resilience

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_StaysClosedOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	for i := 0; i < 10; i++ {
		assert.True(t, cb.Allow())
		cb.RecordSuccess()
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
		assert.True(t, cb.Allow())
		cb.RecordFailure()
	}

	// Circuit should be open now
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow())
}

func TestCircuitBreaker_RejectsWhenOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      1 * time.Hour, // Long timeout to ensure it stays open
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// All subsequent requests should be rejected
	for i := 0; i < 10; i++ {
		assert.False(t, cb.Allow())
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
		cb.Allow()
		cb.RecordFailure()
	}

	assert.Equal(t, StateOpen, cb.State())

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open and allow one request
	assert.True(t, cb.Allow())
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
		cb.Allow()
		cb.RecordFailure()
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Record successes in half-open state
	cb.Allow()
	cb.RecordSuccess()
	cb.Allow()
	cb.RecordSuccess()

	// Should be closed now
	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_ReopensOnFailureInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Allow one request
	cb.Allow()
	assert.Equal(t, StateHalfOpen, cb.State())

	// Record a failure
	cb.RecordFailure()

	// Should be back to open
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow())
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	})

	// Record some failures (but not enough to trip)
	for i := 0; i < 4; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// Record a success
	cb.Allow()
	cb.RecordSuccess()

	// Failure count should be reset, so more failures needed to trip
	for i := 0; i < 4; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// Should still be closed
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_Execute_Success(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	result, err := Execute(cb, func() (string, error) {
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestCircuitBreaker_Execute_Failure(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	expectedErr := errors.New("operation failed")

	result, err := Execute(cb, func() (string, error) {
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
		Execute(cb, func() (string, error) {
			return "", errors.New("fail")
		})
	}

	// Next execution should fail immediately
	_, err := Execute(cb, func() (string, error) {
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
			if cb.Allow() {
				atomic.AddInt64(&successesAllowed, 1)
				cb.RecordSuccess()
			}
		}
		done <- true
	}()

	// Concurrent failures
	go func() {
		for i := 0; i < 50; i++ {
			if cb.Allow() {
				atomic.AddInt64(&failuresAllowed, 1)
				cb.RecordFailure()
			}
		}
		done <- true
	}()

	// Concurrent state checks
	go func() {
		for i := 0; i < 50; i++ {
			cb.State()
			cb.Allow()
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
		cb.Allow()
		cb.RecordSuccess()
	}
	for i := 0; i < 3; i++ {
		cb.Allow()
		cb.RecordFailure()
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
		cb.Allow()
		cb.RecordFailure()
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// First request allowed
	assert.True(t, cb.Allow())
	assert.Equal(t, StateHalfOpen, cb.State())

	// Subsequent requests should be limited until the first completes
	assert.False(t, cb.Allow())
	assert.False(t, cb.Allow())

	// Completing the in-flight trial (success) frees up the slot again
	cb.RecordSuccess()
	assert.True(t, cb.Allow())
}

func TestCircuitBreaker_HalfOpenSlotFreedOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		OpenTimeout:      50 * time.Millisecond,
	})

	// Trip the circuit
	for i := 0; i < 2; i++ {
		cb.Allow()
		cb.RecordFailure()
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// First trial allowed, second rejected while trial is in flight
	assert.True(t, cb.Allow())
	assert.False(t, cb.Allow())

	// Trial fails -> circuit re-opens and the in-flight slot is released
	cb.RecordFailure()
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow())
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
		cb.Allow()
		cb.RecordFailure()
	}
	require.Equal(t, StateOpen, cb.State())

	// Wait for half-open, then take the trial permit and record nothing
	time.Sleep(60 * time.Millisecond)
	require.True(t, cb.Allow())

	// While the trial is still fresh the slot stays reserved
	assert.False(t, cb.Allow())

	// Once the trial has outlived its bound the abandoned permit is reclaimed
	time.Sleep(60 * time.Millisecond)
	assert.True(t, cb.Allow(), "an unresolved trial permit must be reclaimed, not wedge the circuit shut")
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
		cb.Allow()
		cb.RecordFailure()
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	permit, allowed := cb.Acquire()
	require.True(t, allowed)
	assert.Equal(t, 1, cb.Stats().HalfOpenInFlight)

	// Caller returns early without recording an outcome
	permit.Release()
	assert.Equal(t, 0, cb.Stats().HalfOpenInFlight)

	next, allowed := cb.Acquire()
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
		cb.Allow()
		cb.RecordFailure()
	}
	time.Sleep(60 * time.Millisecond)

	first, allowed := cb.Acquire()
	require.True(t, allowed)
	cb.RecordSuccess()

	second, allowed := cb.Acquire()
	require.True(t, allowed)

	// The first permit is stale now; releasing it must not free the second slot
	first.Release()
	assert.Equal(t, 1, cb.Stats().HalfOpenInFlight)
	assert.False(t, cb.Allow(), "a stale Release must not admit a second concurrent trial")

	second.Release()
	assert.True(t, cb.Allow())
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
		permit, allowed := cb.Acquire()
		require.True(t, allowed)
		cb.RecordSuccess()
		permit.Release()
	}
	require.Equal(t, StateClosed, cb.State())
	assert.Equal(t, 0, cb.Stats().HalfOpenInFlight)

	// Closed -> open on consecutive failures
	for i := 0; i < 3; i++ {
		permit, allowed := cb.Acquire()
		require.True(t, allowed)
		cb.RecordFailure()
		permit.Release()
	}
	require.Equal(t, StateOpen, cb.State())
	_, allowed := cb.Acquire()
	assert.False(t, allowed, "open circuit must reject requests")

	// Open -> half-open after OpenTimeout, one trial at a time
	time.Sleep(60 * time.Millisecond)
	trial, allowed := cb.Acquire()
	require.True(t, allowed)
	require.Equal(t, StateHalfOpen, cb.State())
	_, allowed = cb.Acquire()
	assert.False(t, allowed, "half-open must admit only one trial at a time")

	// Half-open -> closed once SuccessThreshold trials succeed
	cb.RecordSuccess()
	trial.Release()
	assert.Equal(t, StateHalfOpen, cb.State())

	trial, allowed = cb.Acquire()
	require.True(t, allowed)
	cb.RecordSuccess()
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
		cb.Allow()
		cb.RecordFailure()
	}
	require.Equal(t, StateOpen, cb.State())
	time.Sleep(320 * time.Millisecond)
	warmup, allowed := cb.Acquire()
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
				permit, allowed := cb.Acquire()
				if !allowed {
					sampleLedger()
					continue
				}
				atomic.AddInt64(&granted, 1)
				sampleLedger()

				// Half the trials reach the dependency and record an outcome;
				// the other half return early (cache hit) and only release.
				if j%2 == 0 {
					cb.RecordSuccess()
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
	assert.True(t, cb.Allow(), "a sound permit ledger must still admit the next trial")
}
