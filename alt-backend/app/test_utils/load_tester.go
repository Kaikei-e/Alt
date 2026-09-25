package test_utils

import (
	"context"
	"fmt"
	"time"
)

// RunLoadTest simulates increasing load over time
func (pt *PerformanceTester) RunLoadTest(operation func() error, loadPattern []LoadStep) (*LoadTestResult, error) {
	pt.logger("Starting load test with %d steps", len(loadPattern))

	loadResult := &LoadTestResult{
		Steps:     make([]LoadStepResult, 0, len(loadPattern)),
		StartTime: time.Now(),
	}

	for i, step := range loadPattern {
		pt.logger("Load test step %d: %d ops/sec for %v", i+1, step.TargetOPS, step.Duration)

		stepResult, err := pt.runLoadStep(operation, step)
		if err != nil {
			return loadResult, fmt.Errorf("load step %d failed: %w", i, err)
		}

		loadResult.Steps = append(loadResult.Steps, *stepResult)

		// Check if we should continue based on error rate
		if stepResult.ErrorRate > 0.1 { // 10% error rate threshold
			pt.logger("Stopping load test due to high error rate: %.2f%%", stepResult.ErrorRate*100)
			break
		}
	}

	loadResult.EndTime = time.Now()
	loadResult.TotalDuration = loadResult.EndTime.Sub(loadResult.StartTime)

	return loadResult, nil
}

type LoadStep struct {
	TargetOPS int
	Duration  time.Duration
}

type LoadTestResult struct {
	Steps         []LoadStepResult
	StartTime     time.Time
	EndTime       time.Time
	TotalDuration time.Duration
}

type LoadStepResult struct {
	TargetOPS      int
	ActualOPS      float64
	Duration       time.Duration
	SuccessfulOps  int
	FailedOps      int
	ErrorRate      float64
	AverageLatency time.Duration
	MaxLatency     time.Duration
}

func (pt *PerformanceTester) runLoadStep(operation func() error, step LoadStep) (*LoadStepResult, error) {
	stepResult := &LoadStepResult{
		TargetOPS: step.TargetOPS,
		Duration:  step.Duration,
	}

	// Calculate interval between operations
	interval := time.Second / time.Duration(step.TargetOPS)

	ctx, cancel := context.WithTimeout(context.Background(), step.Duration)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var latencies []time.Duration

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			// Step duration completed
			stepResult.Duration = time.Since(startTime)
			stepResult.ActualOPS = float64(stepResult.SuccessfulOps+stepResult.FailedOps) / stepResult.Duration.Seconds()

			if len(latencies) > 0 {
				stepResult.AverageLatency = calculateAverage(latencies)
				stepResult.MaxLatency = calculateMax(latencies)
			}

			if stepResult.SuccessfulOps+stepResult.FailedOps > 0 {
				stepResult.ErrorRate = float64(stepResult.FailedOps) / float64(stepResult.SuccessfulOps+stepResult.FailedOps)
			}

			return stepResult, nil

		case <-ticker.C:
			// Execute operation
			opStart := time.Now()
			err := operation()
			latency := time.Since(opStart)

			latencies = append(latencies, latency)

			if err == nil {
				stepResult.SuccessfulOps++
			} else {
				stepResult.FailedOps++
			}
		}
	}
}
