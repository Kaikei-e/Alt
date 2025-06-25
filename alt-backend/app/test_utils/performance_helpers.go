package test_utils

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// PerformanceTestConfig holds configuration for performance tests
type PerformanceTestConfig struct {
	Name                string
	WarmupIterations    int
	TestIterations      int
	ConcurrentWorkers   int
	TimeoutDuration     time.Duration
	MemoryLimitMB       int64
	ThroughputThreshold float64
	LatencyThreshold    time.Duration
}

// DefaultPerformanceConfig returns a default performance test configuration
func DefaultPerformanceConfig() PerformanceTestConfig {
	return PerformanceTestConfig{
		Name:                "default_performance_test",
		WarmupIterations:    100,
		TestIterations:      1000,
		ConcurrentWorkers:   10,
		TimeoutDuration:     30 * time.Second,
		MemoryLimitMB:       100,
		ThroughputThreshold: 100.0, // operations per second
		LatencyThreshold:    100 * time.Millisecond,
	}
}

// PerformanceResult contains the results of a performance test
type PerformanceResult struct {
	TestName           string
	TotalOperations    int
	SuccessfulOps      int
	FailedOps          int
	Duration           time.Duration
	Throughput         float64 // operations per second
	AverageLatency     time.Duration
	MinLatency         time.Duration
	MaxLatency         time.Duration
	P50Latency         time.Duration
	P95Latency         time.Duration
	P99Latency         time.Duration
	MemoryUsage        MemoryUsage
	Errors             []error
}

// MemoryUsage contains memory usage statistics
type MemoryUsage struct {
	AllocMB      float64
	TotalAllocMB float64
	SysMB        float64
	NumGC        uint32
}

// PerformanceTester provides utilities for running performance tests
type PerformanceTester struct {
	config PerformanceTestConfig
	logger func(format string, args ...interface{})
}

// NewPerformanceTester creates a new performance tester
func NewPerformanceTester(config PerformanceTestConfig) *PerformanceTester {
	return &PerformanceTester{
		config: config,
		logger: func(format string, args ...interface{}) {
			// Default logger - can be overridden
			fmt.Printf("[PERF] "+format+"\n", args...)
		},
	}
}

// SetLogger sets a custom logger for the performance tester
func (pt *PerformanceTester) SetLogger(logger func(format string, args ...interface{})) {
	pt.logger = logger
}

// RunTest executes a performance test with the given operation
func (pt *PerformanceTester) RunTest(operation func() error) (*PerformanceResult, error) {
	pt.logger("Starting performance test: %s", pt.config.Name)
	
	// Warmup phase
	if err := pt.runWarmup(operation); err != nil {
		return nil, fmt.Errorf("warmup failed: %w", err)
	}
	
	// Collect initial memory stats
	runtime.GC()
	initialMemory := getMemoryUsage()
	
	// Main test phase
	result, err := pt.runMainTest(operation)
	if err != nil {
		return nil, fmt.Errorf("main test failed: %w", err)
	}
	
	// Collect final memory stats
	runtime.GC()
	finalMemory := getMemoryUsage()
	result.MemoryUsage = calculateMemoryDelta(initialMemory, finalMemory)
	
	// Validate results
	if err := pt.validateResults(result); err != nil {
		return result, fmt.Errorf("performance validation failed: %w", err)
	}
	
	pt.logger("Performance test completed: %s", pt.config.Name)
	return result, nil
}

func (pt *PerformanceTester) runWarmup(operation func() error) error {
	pt.logger("Running warmup: %d iterations", pt.config.WarmupIterations)
	
	for i := 0; i < pt.config.WarmupIterations; i++ {
		if err := operation(); err != nil {
			return fmt.Errorf("warmup iteration %d failed: %w", i, err)
		}
	}
	
	return nil
}

func (pt *PerformanceTester) runMainTest(operation func() error) (*PerformanceResult, error) {
	pt.logger("Running main test: %d operations with %d workers", 
		pt.config.TestIterations, pt.config.ConcurrentWorkers)
	
	result := &PerformanceResult{
		TestName: pt.config.Name,
		Errors:   make([]error, 0),
	}
	
	// Create channels for coordination
	tasks := make(chan int, pt.config.TestIterations)
	results := make(chan operationResult, pt.config.TestIterations)
	
	// Fill task channel
	for i := 0; i < pt.config.TestIterations; i++ {
		tasks <- i
	}
	close(tasks)
	
	// Start timer
	startTime := time.Now()
	
	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < pt.config.ConcurrentWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			pt.worker(workerID, tasks, results, operation)
		}(i)
	}
	
	// Wait for completion with timeout
	done := make(chan bool)
	go func() {
		wg.Wait()
		close(results)
		done <- true
	}()
	
	select {
	case <-done:
		// Test completed normally
	case <-time.After(pt.config.TimeoutDuration):
		return nil, fmt.Errorf("test timed out after %v", pt.config.TimeoutDuration)
	}
	
	// Calculate results
	result.Duration = time.Since(startTime)
	return pt.calculateResults(result, results), nil
}

type operationResult struct {
	success bool
	latency time.Duration
	error   error
}

func (pt *PerformanceTester) worker(workerID int, tasks <-chan int, results chan<- operationResult, operation func() error) {
	for taskID := range tasks {
		start := time.Now()
		err := operation()
		latency := time.Since(start)
		
		results <- operationResult{
			success: err == nil,
			latency: latency,
			error:   err,
		}
		
		// Log progress for very long tests
		if taskID%1000 == 0 && taskID > 0 {
			pt.logger("Worker %d completed task %d", workerID, taskID)
		}
	}
}

func (pt *PerformanceTester) calculateResults(result *PerformanceResult, results <-chan operationResult) *PerformanceResult {
	var latencies []time.Duration
	
	for opResult := range results {
		result.TotalOperations++
		latencies = append(latencies, opResult.latency)
		
		if opResult.success {
			result.SuccessfulOps++
		} else {
			result.FailedOps++
			if opResult.error != nil {
				result.Errors = append(result.Errors, opResult.error)
			}
		}
	}
	
	// Calculate throughput
	result.Throughput = float64(result.SuccessfulOps) / result.Duration.Seconds()
	
	// Calculate latency statistics
	if len(latencies) > 0 {
		result.AverageLatency = calculateAverage(latencies)
		result.MinLatency = calculateMin(latencies)
		result.MaxLatency = calculateMax(latencies)
		result.P50Latency = calculatePercentile(latencies, 50)
		result.P95Latency = calculatePercentile(latencies, 95)
		result.P99Latency = calculatePercentile(latencies, 99)
	}
	
	return result
}

func (pt *PerformanceTester) validateResults(result *PerformanceResult) error {
	var validationErrors []string
	
	// Check throughput threshold
	if result.Throughput < pt.config.ThroughputThreshold {
		validationErrors = append(validationErrors, 
			fmt.Sprintf("throughput %.2f < threshold %.2f ops/sec", 
				result.Throughput, pt.config.ThroughputThreshold))
	}
	
	// Check latency threshold
	if result.AverageLatency > pt.config.LatencyThreshold {
		validationErrors = append(validationErrors, 
			fmt.Sprintf("average latency %v > threshold %v", 
				result.AverageLatency, pt.config.LatencyThreshold))
	}
	
	// Check memory usage
	if pt.config.MemoryLimitMB > 0 && result.MemoryUsage.AllocMB > float64(pt.config.MemoryLimitMB) {
		validationErrors = append(validationErrors, 
			fmt.Sprintf("memory usage %.2f MB > limit %d MB", 
				result.MemoryUsage.AllocMB, pt.config.MemoryLimitMB))
	}
	
	// Check error rate
	errorRate := float64(result.FailedOps) / float64(result.TotalOperations)
	if errorRate > 0.05 { // 5% error rate threshold
		validationErrors = append(validationErrors, 
			fmt.Sprintf("error rate %.2f%% > 5%%", errorRate*100))
	}
	
	if len(validationErrors) > 0 {
		return fmt.Errorf("performance validation failed: %v", validationErrors)
	}
	
	return nil
}

// RunConcurrencyTest tests performance under various concurrency levels
func (pt *PerformanceTester) RunConcurrencyTest(operation func() error, concurrencyLevels []int) (map[int]*PerformanceResult, error) {
	results := make(map[int]*PerformanceResult)
	
	for _, level := range concurrencyLevels {
		pt.logger("Testing concurrency level: %d", level)
		
		// Update config for this test
		originalWorkers := pt.config.ConcurrentWorkers
		pt.config.ConcurrentWorkers = level
		pt.config.Name = fmt.Sprintf("%s_concurrency_%d", pt.config.Name, level)
		
		result, err := pt.RunTest(operation)
		if err != nil {
			pt.config.ConcurrentWorkers = originalWorkers
			return results, fmt.Errorf("concurrency test failed at level %d: %w", level, err)
		}
		
		results[level] = result
		pt.config.ConcurrentWorkers = originalWorkers
	}
	
	return results, nil
}

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

// Utility functions for performance calculations
func getMemoryUsage() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

func calculateMemoryDelta(initial, final runtime.MemStats) MemoryUsage {
	return MemoryUsage{
		AllocMB:      float64(final.Alloc-initial.Alloc) / 1024 / 1024,
		TotalAllocMB: float64(final.TotalAlloc-initial.TotalAlloc) / 1024 / 1024,
		SysMB:        float64(final.Sys-initial.Sys) / 1024 / 1024,
		NumGC:        final.NumGC - initial.NumGC,
	}
}

func calculateAverage(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	
	return total / time.Duration(len(durations))
}

func calculateMin(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	
	min := durations[0]
	for _, d := range durations[1:] {
		if d < min {
			min = d
		}
	}
	
	return min
}

func calculateMax(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	
	max := durations[0]
	for _, d := range durations[1:] {
		if d > max {
			max = d
		}
	}
	
	return max
}

func calculatePercentile(durations []time.Duration, percentile int) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	
	// Simple percentile calculation (for production, use a proper sorting algorithm)
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	
	// Bubble sort (simple but not efficient for large datasets)
	for i := 0; i < len(sorted); i++ {
		for j := 0; j < len(sorted)-1; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}
	
	index := (percentile * len(sorted)) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	
	return sorted[index]
}

// BenchmarkHelper provides utilities for Go benchmark tests
type BenchmarkHelper struct {
	b *testing.B
}

func NewBenchmarkHelper(b *testing.B) *BenchmarkHelper {
	return &BenchmarkHelper{b: b}
}

func (bh *BenchmarkHelper) RunParallelBenchmark(operation func(pb *testing.PB)) {
	bh.b.ResetTimer()
	bh.b.RunParallel(operation)
}

func (bh *BenchmarkHelper) MeasureMemoryAllocations(operation func()) {
	bh.b.ResetTimer()
	bh.b.ReportAllocs()
	
	for i := 0; i < bh.b.N; i++ {
		operation()
	}
}

func (bh *BenchmarkHelper) BenchmarkWithSetup(setup func(), operation func(), teardown func()) {
	for i := 0; i < bh.b.N; i++ {
		bh.b.StopTimer()
		setup()
		bh.b.StartTimer()
		
		operation()
		
		bh.b.StopTimer()
		teardown()
	}
}