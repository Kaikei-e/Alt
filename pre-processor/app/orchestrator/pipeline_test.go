package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunStage_BasicConcurrency(t *testing.T) {
	t.Run("should process all inputs and return ordered results", func(t *testing.T) {
		inputs := []int{1, 2, 3, 4, 5}

		results := RunStage(context.Background(), Stage[int, int]{
			Name:        "double",
			Concurrency: 3,
			Process: func(_ context.Context, in int) (int, error) {
				return in * 2, nil
			},
		}, inputs)

		require.Len(t, results, 5)
		for i, r := range results {
			assert.NoError(t, r.Err)
			assert.Equal(t, inputs[i]*2, r.Value)
			assert.Equal(t, i, r.Index)
		}
	})
}

func TestRunStage_EmptyInput(t *testing.T) {
	t.Run("should return nil for empty input", func(t *testing.T) {
		results := RunStage(context.Background(), Stage[int, int]{
			Name:        "noop",
			Concurrency: 3,
			Process: func(_ context.Context, in int) (int, error) {
				return in, nil
			},
		}, nil)

		assert.Nil(t, results)
	})
}

func TestRunStage_ErrorHandling(t *testing.T) {
	t.Run("should capture errors per item without stopping others", func(t *testing.T) {
		errBoom := errors.New("boom")
		inputs := []int{1, 2, 3}

		results := RunStage(context.Background(), Stage[int, int]{
			Name:        "maybe-fail",
			Concurrency: 3,
			Process: func(_ context.Context, in int) (int, error) {
				if in == 2 {
					return 0, errBoom
				}
				return in * 10, nil
			},
		}, inputs)

		require.Len(t, results, 3)
		assert.NoError(t, results[0].Err)
		assert.Equal(t, 10, results[0].Value)
		assert.ErrorIs(t, results[1].Err, errBoom)
		assert.NoError(t, results[2].Err)
		assert.Equal(t, 30, results[2].Value)
	})
}

func TestRunStage_BoundsConcurrency(t *testing.T) {
	t.Run("should limit concurrent workers to configured value", func(t *testing.T) {
		var maxConcurrent atomic.Int32
		var current atomic.Int32

		inputs := make([]int, 20)
		for i := range inputs {
			inputs[i] = i
		}

		_ = RunStage(context.Background(), Stage[int, int]{
			Name:        "track-concurrency",
			Concurrency: 3,
			Process: func(_ context.Context, in int) (int, error) {
				c := current.Add(1)
				// Track the max concurrent value
				for {
					old := maxConcurrent.Load()
					if c <= old || maxConcurrent.CompareAndSwap(old, c) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond) // Simulate work
				current.Add(-1)
				return in, nil
			},
		}, inputs)

		assert.LessOrEqual(t, maxConcurrent.Load(), int32(3),
			"max concurrent workers should not exceed configured concurrency")
		assert.Greater(t, maxConcurrent.Load(), int32(1),
			"should actually use concurrent workers")
	})
}

func TestRunStage_ContextCancellation(t *testing.T) {
	t.Run("should respect context cancellation in process function", func(t *testing.T) {
		inputs := make([]int, 10)
		for i := range inputs {
			inputs[i] = i
		}

		ctx, cancel := context.WithCancel(context.Background())

		// RunStage launches all len(inputs) goroutines up front; with
		// Concurrency: 1 they still race each other to acquire the
		// capacity-1 semaphore, so which input VALUE (e.g. in==2) ends up
		// executing Nth is not guaranteed — triggering cancellation from a
		// specific input value made this test intermittently fail (0
		// cancellations observed) whenever that goroutine happened to win
		// the race last instead of early. Concurrency: 1 does guarantee
		// Process calls are serialized (mutual exclusion via the
		// semaphore), so triggering cancellation by call *count* instead is
		// deterministic regardless of scheduling order.
		var processed atomic.Int32
		results := RunStage(ctx, Stage[int, int]{
			Name:        "cancelable",
			Concurrency: 1,
			Process: func(ctx context.Context, in int) (int, error) {
				if ctx.Err() != nil {
					return 0, ctx.Err()
				}
				if processed.Add(1) == 5 {
					cancel()
				}
				return in, nil
			},
		}, inputs)

		require.Len(t, results, 10, "RunStage always returns len(inputs) results")

		// The 5th serialized call cancels the context; every later call is
		// guaranteed (by the semaphore's mutual exclusion) to observe
		// ctx.Err() != nil, either in RunStage's own pre-Process check or in
		// this Process func's own check — so exactly the remaining 5 of 10
		// results must carry the cancellation error, deterministically.
		var cancelledCount int
		for _, r := range results {
			if r.Err != nil {
				assert.ErrorIs(t, r.Err, context.Canceled)
				cancelledCount++
			}
		}
		assert.Equal(t, 5, cancelledCount,
			"exactly the calls after the 5th (serialized) one should observe cancellation")
	})
}

func TestRunStage_ConcurrencyExceedsInputs(t *testing.T) {
	t.Run("should handle concurrency greater than input size", func(t *testing.T) {
		inputs := []string{"a", "b"}

		results := RunStage(context.Background(), Stage[string, string]{
			Name:        "high-concurrency",
			Concurrency: 100,
			Process: func(_ context.Context, in string) (string, error) {
				return in + "!", nil
			},
		}, inputs)

		require.Len(t, results, 2)
		assert.Equal(t, "a!", results[0].Value)
		assert.Equal(t, "b!", results[1].Value)
	})
}
