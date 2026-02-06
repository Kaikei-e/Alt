package orchestrator

import (
	"context"
	"sync"
)

// Result wraps the output of a pipeline stage with its error.
type Result[Out any] struct {
	Value Out
	Err   error
	Index int // Original index in the input slice
}

// Stage defines a concurrent processing stage.
type Stage[In, Out any] struct {
	Name        string
	Concurrency int // Maximum number of concurrent workers
	Process     func(ctx context.Context, input In) (Out, error)
}

// RunStage executes the stage's Process function over all inputs with bounded concurrency.
// Results are returned in the same order as inputs.
func RunStage[In, Out any](ctx context.Context, stage Stage[In, Out], inputs []In) []Result[Out] {
	if len(inputs) == 0 {
		return nil
	}

	concurrency := stage.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(inputs) {
		concurrency = len(inputs)
	}

	results := make([]Result[Out], len(inputs))
	sem := make(chan struct{}, concurrency)

	var wg sync.WaitGroup
	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in In) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = Result[Out]{Err: ctx.Err(), Index: idx}
				return
			}

			// Check context before processing
			if ctx.Err() != nil {
				results[idx] = Result[Out]{Err: ctx.Err(), Index: idx}
				return
			}

			out, err := stage.Process(ctx, in)
			results[idx] = Result[Out]{Value: out, Err: err, Index: idx}
		}(i, input)
	}

	wg.Wait()
	return results
}
