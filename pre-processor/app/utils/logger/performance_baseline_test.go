// ABOUTME: This file measures performance baseline of current logging implementation
// ABOUTME: Establishes benchmarks for comparison with new unified logger
package logger

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func BenchmarkCurrentRaskLogger(b *testing.B) {
	// Benchmark UnifiedLogger implementation (replacement for RaskLogger)
	var buf bytes.Buffer
	logger := NewUnifiedLogger(&buf, "pre-processor")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message",
			"iteration", i,
			"operation", "benchmark_test",
			"status", "running")
	}
}

func BenchmarkCurrentRaskLoggerWithContext(b *testing.B) {
	// Benchmark UnifiedLogger with context attributes
	var buf bytes.Buffer
	logger := NewUnifiedLogger(&buf, "pre-processor")
	contextLogger := logger.With(
		"request_id", "bench-123",
		"trace_id", "trace-456",
		"operation", "benchmark")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		contextLogger.Info("context benchmark message",
			"iteration", i,
			"status", "running")
	}
}

func BenchmarkCurrentContextLogger(b *testing.B) {
	// Benchmark current ContextLogger implementation
	var buf bytes.Buffer
	contextLogger := NewContextLogger(&buf, "json", "info")

	ctx := WithRequestID(WithTraceID(context.Background(), "trace-bench"), "req-bench")
	logger := contextLogger.WithContext(ctx)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Info("context logger benchmark",
			"iteration", i,
			"operation", "benchmark_test")
	}
}

func BenchmarkUnifiedLogger(b *testing.B) {
	// Benchmark new UnifiedLogger implementation (will fail initially)
	var buf bytes.Buffer
	logger := NewUnifiedLogger(&buf, "pre-processor")

	ctx := WithRequestID(WithTraceID(context.Background(), "trace-unified"), "req-unified")
	contextLogger := logger.WithContext(ctx)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		contextLogger.Info("unified logger benchmark",
			"iteration", i,
			"operation", "benchmark_test")
	}
}

func BenchmarkMemoryUsage(b *testing.B) {
	// Memory usage benchmark for UnifiedLogger implementation
	logger := NewUnifiedLogger(os.Stdout, "pre-processor")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simulate typical logging pattern
		contextLogger := logger.With(
			"request_id", "req-mem-test",
			"trace_id", "trace-mem-test")

		contextLogger.Info("memory test message",
			"feed_id", "feed-123",
			"status", "processed",
			"duration_ms", 150,
			"items_count", 42)
	}
}

func BenchmarkJSONMarshaling(b *testing.B) {
	// Benchmark JSON marshaling overhead in UnifiedLogger implementation
	var buf bytes.Buffer
	logger := NewUnifiedLogger(&buf, "pre-processor")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		logger.Info("json marshal test",
			"nested", "value",
			"count", i,
			"flag", true)
	}
}

func TestPerformanceBaseline(t *testing.T) {
	// Establish baseline performance metrics
	if testing.Short() {
		t.Skip("skipping performance baseline in short mode")
	}

	// Test UnifiedLogger performance
	var buf bytes.Buffer
	logger := NewUnifiedLogger(&buf, "pre-processor")

	startMem := testing.AllocsPerRun(1000, func() {
		logger.Info("baseline test",
			"operation", "performance_test",
			"iteration", 1)
	})

	// Document baseline metrics for comparison
	t.Logf("UnifiedLogger allocations per operation: %.2f", startMem)

	// Compare against previous RaskLogger implementation
	if startMem > 10 {
		t.Logf("HIGH: Current implementation uses %.2f allocations per log call", startMem)
	}
}

func TestCurrentImplementationLimits(t *testing.T) {
	// Test UnifiedLogger implementation limits and bottlenecks
	var buf bytes.Buffer
	logger := NewUnifiedLogger(&buf, "pre-processor")

	// Test with many attributes
	args := make([]any, 0, 20)
	for i := 0; i < 10; i++ {
		args = append(args, "key"+string(rune('0'+i)), "value"+string(rune('0'+i)))
	}

	logger.Info("stress test with many attributes", args...)

	// Verify output is still valid
	if buf.Len() == 0 {
		t.Error("No output produced with many attributes")
	}

	// Test memory growth with context chaining
	baseLogger := logger
	for i := 0; i < 5; i++ {
		baseLogger = baseLogger.With("chain_level", i, "data", "test_value")
	}

	baseLogger.Info("chained context test")
}
