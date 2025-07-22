// Phase R4: パフォーマンステスト - 新アーキテクチャの性能検証
package infrastructure

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"deploy-cli/infrastructure/config"
	"deploy-cli/infrastructure/container"
	"deploy-cli/infrastructure/logging"
)

// BenchmarkResults holds performance test results
type BenchmarkResults struct {
	TestName          string
	TotalOperations   int64
	Duration          time.Duration
	OperationsPerSec  float64
	AvgLatency        time.Duration
	P95Latency        time.Duration
	P99Latency        time.Duration
	MemoryAllocated   uint64
	MemoryAllocations int64
	Errors            int64
}

// PerformanceTestSuite runs comprehensive performance tests
type PerformanceTestSuite struct {
	infra   *InfrastructureContainer
	results []BenchmarkResults
	mutex   sync.RWMutex
}

// NewPerformanceTestSuite creates a new performance test suite
func NewPerformanceTestSuite() (*PerformanceTestSuite, error) {
	infra, err := NewInfrastructureContainer(config.Development)
	if err != nil {
		return nil, fmt.Errorf("failed to create infrastructure: %w", err)
	}

	return &PerformanceTestSuite{
		infra:   infra,
		results: make([]BenchmarkResults, 0),
	}, nil
}

// RunAllBenchmarks runs all performance benchmarks
func (pts *PerformanceTestSuite) RunAllBenchmarks(t *testing.T) {
	t.Run("DependencyInjection", pts.BenchmarkDependencyInjection)
	t.Run("ConfigurationAccess", pts.BenchmarkConfigurationAccess)
	t.Run("StructuredLogging", pts.BenchmarkStructuredLogging)
	t.Run("LogContextManagement", pts.BenchmarkLogContextManagement)
	t.Run("ServiceRegistration", pts.BenchmarkServiceRegistration)
	t.Run("ConcurrentOperations", pts.BenchmarkConcurrentOperations)
	t.Run("MemoryUsage", pts.BenchmarkMemoryUsage)
}

// BenchmarkDependencyInjection benchmarks DI container performance
func (pts *PerformanceTestSuite) BenchmarkDependencyInjection(t *testing.T) {
	container := container.NewDependencyContainer()
	
	// Register test services
	container.RegisterSingleton((*TestService)(nil), func() *TestService {
		return &TestService{ID: "test"}
	})
	
	container.RegisterTransient((*TestTransientService)(nil), func(ts *TestService) *TestTransientService {
		return &TestTransientService{Service: ts}
	})

	result := pts.runBenchmark("DependencyInjection", func() error {
		_, err := container.Resolve((*TestTransientService)(nil))
		return err
	}, 10000)

	pts.addResult(result)
	t.Logf("DI Performance: %v ops/sec, avg latency: %v", result.OperationsPerSec, result.AvgLatency)
}

// BenchmarkConfigurationAccess benchmarks config access performance
func (pts *PerformanceTestSuite) BenchmarkConfigurationAccess(t *testing.T) {
	result := pts.runBenchmark("ConfigurationAccess", func() error {
		_ = pts.infra.configManager.GetString("logging.level")
		_ = pts.infra.configManager.GetBool("deployment.parallel.enabled")
		_ = pts.infra.configManager.GetInt("deployment.parallel.max_workers")
		_ = pts.infra.configManager.GetDuration("helm.timeout")
		return nil
	}, 50000)

	pts.addResult(result)
	t.Logf("Config Access Performance: %v ops/sec, avg latency: %v", result.OperationsPerSec, result.AvgLatency)
}

// BenchmarkStructuredLogging benchmarks structured logging performance
func (pts *PerformanceTestSuite) BenchmarkStructuredLogging(t *testing.T) {
	ctx := context.Background()
	
	result := pts.runBenchmark("StructuredLogging", func() error {
		pts.infra.structLogger.Info(ctx, "Performance test message",
			"operation", "test",
			"count", 1,
			"timestamp", time.Now(),
			"data", map[string]interface{}{
				"key1": "value1",
				"key2": 42,
				"key3": true,
			},
		)
		return nil
	}, 25000)

	pts.addResult(result)
	t.Logf("Structured Logging Performance: %v ops/sec, avg latency: %v", result.OperationsPerSec, result.AvgLatency)
}

// BenchmarkLogContextManagement benchmarks log context management
func (pts *PerformanceTestSuite) BenchmarkLogContextManagement(t *testing.T) {
	result := pts.runBenchmark("LogContextManagement", func() error {
		ctx := context.Background()
		newCtx, span := pts.infra.contextManager.StartOperation(ctx, "test_operation")
		
		pts.infra.contextManager.AddSpanTag(newCtx, "test_key", "test_value")
		pts.infra.contextManager.LogToSpan(newCtx, logging.InfoLevel, "test message", map[string]interface{}{
			"test": "data",
		})
		
		pts.infra.contextManager.FinishOperation(newCtx, span, nil)
		return nil
	}, 10000)

	pts.addResult(result)
	t.Logf("Context Management Performance: %v ops/sec, avg latency: %v", result.OperationsPerSec, result.AvgLatency)
}

// BenchmarkServiceRegistration benchmarks service registration performance
func (pts *PerformanceTestSuite) BenchmarkServiceRegistration(t *testing.T) {
	registry := container.NewServiceRegistry(container.NewDependencyContainer())
	
	result := pts.runBenchmark("ServiceRegistration", func() error {
		return registry.RegisterService(&container.ServiceRegistrationOptions{
			ServiceType: (*TestService)(nil),
			Factory: func() *TestService {
				return &TestService{ID: fmt.Sprintf("service_%d", time.Now().UnixNano())}
			},
			Lifecycle: container.Transient,
		})
	}, 5000)

	pts.addResult(result)
	t.Logf("Service Registration Performance: %v ops/sec, avg latency: %v", result.OperationsPerSec, result.AvgLatency)
}

// BenchmarkConcurrentOperations benchmarks concurrent operations
func (pts *PerformanceTestSuite) BenchmarkConcurrentOperations(t *testing.T) {
	const numGoroutines = 50
	const operationsPerGoroutine = 1000

	start := time.Now()
	var wg sync.WaitGroup
	var errors int64
	var totalOps int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			
			for j := 0; j < operationsPerGoroutine; j++ {
				// Mix of operations
				switch j % 4 {
				case 0:
					pts.infra.configManager.GetString("logging.level")
				case 1:
					newCtx, span := pts.infra.contextManager.StartOperation(ctx, "concurrent_test")
					pts.infra.contextManager.FinishOperation(newCtx, span, nil)
				case 2:
					pts.infra.structLogger.Info(ctx, "Concurrent test message", "iteration", j)
				case 3:
					_, err := pts.infra.serviceRegistry.ResolveService((*config.ConfigManager)(nil))
					if err != nil {
						errors++
					}
				}
				totalOps++
			}
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	result := BenchmarkResults{
		TestName:         "ConcurrentOperations",
		TotalOperations:  totalOps,
		Duration:         duration,
		OperationsPerSec: float64(totalOps) / duration.Seconds(),
		AvgLatency:       time.Duration(int64(duration) / totalOps),
		Errors:           errors,
	}

	pts.addResult(result)
	t.Logf("Concurrent Operations Performance: %v ops/sec, %v errors", result.OperationsPerSec, result.Errors)
}

// BenchmarkMemoryUsage benchmarks memory usage
func (pts *PerformanceTestSuite) BenchmarkMemoryUsage(t *testing.T) {
	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	const operations = 10000
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < operations; i++ {
		// Perform various operations that allocate memory
		pts.infra.structLogger.WithFields(map[string]interface{}{
			"operation": fmt.Sprintf("test_%d", i),
			"data": map[string]interface{}{
				"nested": map[string]interface{}{
					"value": i,
					"timestamp": time.Now(),
				},
			},
		}).Info(ctx, "Memory test", "iteration", i)

		if i%1000 == 0 {
			runtime.GC()
		}
	}
	duration := time.Since(start)

	runtime.GC()
	runtime.ReadMemStats(&m2)

	result := BenchmarkResults{
		TestName:          "MemoryUsage",
		TotalOperations:   operations,
		Duration:          duration,
		OperationsPerSec:  float64(operations) / duration.Seconds(),
		MemoryAllocated:   m2.TotalAlloc - m1.TotalAlloc,
		MemoryAllocations: int64(m2.Mallocs - m1.Mallocs),
	}

	pts.addResult(result)
	t.Logf("Memory Usage: %v bytes allocated, %v allocations, %v bytes/op",
		result.MemoryAllocated, result.MemoryAllocations, 
		result.MemoryAllocated/uint64(result.TotalOperations))
}

// runBenchmark runs a benchmark function and collects metrics
func (pts *PerformanceTestSuite) runBenchmark(name string, fn func() error, iterations int) BenchmarkResults {
	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	latencies := make([]time.Duration, iterations)
	var errors int64

	start := time.Now()
	for i := 0; i < iterations; i++ {
		opStart := time.Now()
		if err := fn(); err != nil {
			errors++
		}
		latencies[i] = time.Since(opStart)
	}
	duration := time.Since(start)

	runtime.ReadMemStats(&m2)

	// Calculate percentiles
	sortedLatencies := make([]time.Duration, len(latencies))
	copy(sortedLatencies, latencies)
	
	// Simple sort for percentile calculation
	for i := 0; i < len(sortedLatencies); i++ {
		for j := i + 1; j < len(sortedLatencies); j++ {
			if sortedLatencies[i] > sortedLatencies[j] {
				sortedLatencies[i], sortedLatencies[j] = sortedLatencies[j], sortedLatencies[i]
			}
		}
	}

	p95Index := int(float64(len(sortedLatencies)) * 0.95)
	p99Index := int(float64(len(sortedLatencies)) * 0.99)

	return BenchmarkResults{
		TestName:          name,
		TotalOperations:   int64(iterations),
		Duration:          duration,
		OperationsPerSec:  float64(iterations) / duration.Seconds(),
		AvgLatency:        time.Duration(int64(duration) / int64(iterations)),
		P95Latency:        sortedLatencies[p95Index],
		P99Latency:        sortedLatencies[p99Index],
		MemoryAllocated:   m2.TotalAlloc - m1.TotalAlloc,
		MemoryAllocations: int64(m2.Mallocs - m1.Mallocs),
		Errors:            errors,
	}
}

// addResult adds a benchmark result to the collection
func (pts *PerformanceTestSuite) addResult(result BenchmarkResults) {
	pts.mutex.Lock()
	defer pts.mutex.Unlock()
	pts.results = append(pts.results, result)
}

// GetResults returns all benchmark results
func (pts *PerformanceTestSuite) GetResults() []BenchmarkResults {
	pts.mutex.RLock()
	defer pts.mutex.RUnlock()
	
	results := make([]BenchmarkResults, len(pts.results))
	copy(results, pts.results)
	return results
}

// GenerateReport generates a performance report
func (pts *PerformanceTestSuite) GenerateReport() string {
	results := pts.GetResults()
	report := fmt.Sprintf("# Performance Test Report\n\n")
	report += fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339))

	report += "## Summary\n\n"
	report += "| Test Name | Operations | Duration | Ops/Sec | Avg Latency | P95 Latency | P99 Latency | Memory (MB) | Errors |\n"
	report += "|-----------|------------|----------|---------|-------------|-------------|-------------|-------------|--------|\n"

	for _, result := range results {
		memoryMB := float64(result.MemoryAllocated) / 1024 / 1024
		report += fmt.Sprintf("| %s | %d | %s | %.2f | %s | %s | %s | %.2f | %d |\n",
			result.TestName,
			result.TotalOperations,
			result.Duration.String(),
			result.OperationsPerSec,
			result.AvgLatency.String(),
			result.P95Latency.String(),
			result.P99Latency.String(),
			memoryMB,
			result.Errors,
		)
	}

	report += "\n## Performance Analysis\n\n"
	
	// Add analysis based on results
	for _, result := range results {
		report += fmt.Sprintf("### %s\n", result.TestName)
		report += fmt.Sprintf("- Throughput: %.2f operations/second\n", result.OperationsPerSec)
		report += fmt.Sprintf("- Latency: avg=%s, p95=%s, p99=%s\n", 
			result.AvgLatency, result.P95Latency, result.P99Latency)
		report += fmt.Sprintf("- Memory: %.2f MB allocated, %d allocations\n", 
			float64(result.MemoryAllocated)/1024/1024, result.MemoryAllocations)
		
		if result.Errors > 0 {
			report += fmt.Sprintf("- **⚠️ Errors**: %d errors occurred during testing\n", result.Errors)
		}
		
		// Performance assessment
		if result.OperationsPerSec > 10000 {
			report += "- **✅ Performance**: Excellent\n"
		} else if result.OperationsPerSec > 1000 {
			report += "- **✅ Performance**: Good\n"
		} else {
			report += "- **⚠️ Performance**: Needs optimization\n"
		}
		
		report += "\n"
	}

	return report
}

// Cleanup cleans up test resources
func (pts *PerformanceTestSuite) Cleanup() error {
	return pts.infra.Shutdown(context.Background())
}

// Test service types for benchmarking

type TestService struct {
	ID string
}

type TestTransientService struct {
	Service *TestService
}

// Benchmark function that can be called from Go test
func BenchmarkInfrastructurePerformance(b *testing.B) {
	suite, err := NewPerformanceTestSuite()
	if err != nil {
		b.Fatalf("Failed to create performance test suite: %v", err)
	}
	defer suite.Cleanup()

	b.Run("DependencyInjection", func(b *testing.B) {
		container := container.NewDependencyContainer()
		container.RegisterSingleton((*TestService)(nil), func() *TestService {
			return &TestService{ID: "test"}
		})

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := container.Resolve((*TestService)(nil))
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ConfigurationAccess", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			suite.infra.configManager.GetString("logging.level")
		}
	})

	b.Run("StructuredLogging", func(b *testing.B) {
		ctx := context.Background()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			suite.infra.structLogger.Info(ctx, "Benchmark message", "iteration", i)
		}
	})
}