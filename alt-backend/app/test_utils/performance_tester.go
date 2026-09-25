package test_utils

import (
	"fmt"
	"runtime"
	"sync"
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

// PerformanceResult contains the results of a performance test
type PerformanceResult struct {
	TestName        string
	TotalOperations int
	SuccessfulOps   int
	FailedOps       int
	Duration        time.Duration
	Throughput      float64 // operations per second
	AverageLatency  time.Duration
	MinLatency      time.Duration
	MaxLatency      time.Duration
	P50Latency      time.Duration
	P95Latency      time.Duration
	P99Latency      time.Duration
	MemoryUsage     MemoryUsage
	Errors          []error
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
