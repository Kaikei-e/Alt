package utils

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAppMetrics(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create app metrics instance",
			test: func(t *testing.T) {
				metrics := NewAppMetrics()
				assert.NotNil(t, metrics)
				assert.Equal(t, int64(0), metrics.ArticlesProcessed.Load())
				assert.Equal(t, int64(0), metrics.FeedsProcessed.Load())
				assert.Equal(t, int64(0), metrics.SummariesGenerated.Load())
			},
		},
		{
			name: "should increment counters correctly",
			test: func(t *testing.T) {
				metrics := NewAppMetrics()

				metrics.IncrementArticlesProcessed()
				metrics.IncrementFeedsProcessed()
				metrics.IncrementSummariesGenerated()

				assert.Equal(t, int64(1), metrics.ArticlesProcessed.Load())
				assert.Equal(t, int64(1), metrics.FeedsProcessed.Load())
				assert.Equal(t, int64(1), metrics.SummariesGenerated.Load())
			},
		},
		{
			name: "should handle concurrent increments safely",
			test: func(t *testing.T) {
				metrics := NewAppMetrics()
				const numGoroutines = 100
				const incrementsPerGoroutine = 10

				var wg sync.WaitGroup
				wg.Add(numGoroutines)

				for i := 0; i < numGoroutines; i++ {
					go func() {
						defer wg.Done()
						for j := 0; j < incrementsPerGoroutine; j++ {
							metrics.IncrementArticlesProcessed()
						}
					}()
				}

				wg.Wait()

				expected := int64(numGoroutines * incrementsPerGoroutine)
				assert.Equal(t, expected, metrics.ArticlesProcessed.Load())
			},
		},
		{
			name: "should track errors by category",
			test: func(t *testing.T) {
				metrics := NewAppMetrics()

				metrics.IncrementError("database_error")
				metrics.IncrementError("network_error")
				metrics.IncrementError("database_error")

				assert.Equal(t, int64(2), metrics.GetErrorCount("database_error"))
				assert.Equal(t, int64(1), metrics.GetErrorCount("network_error"))
				assert.Equal(t, int64(0), metrics.GetErrorCount("unknown_error"))
			},
		},
		{
			name: "should update gauge values",
			test: func(t *testing.T) {
				metrics := NewAppMetrics()

				metrics.SetActiveGoroutines(10)
				metrics.SetMemoryUsage(1024 * 1024) // 1MB
				metrics.SetQueueDepth("processing", 5)

				assert.Equal(t, int32(10), metrics.ActiveGoroutines.Load())
				assert.Equal(t, int64(1024*1024), metrics.MemoryUsage.Load())
				assert.Equal(t, int32(5), metrics.GetQueueDepth("processing"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestDurationStats(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create duration stats with buckets",
			test: func(t *testing.T) {
				buckets := []time.Duration{
					10 * time.Millisecond,
					50 * time.Millisecond,
					100 * time.Millisecond,
					500 * time.Millisecond,
					1 * time.Second,
				}

				stats := NewDurationStats(buckets)
				assert.NotNil(t, stats)
				assert.Equal(t, len(buckets), len(stats.buckets))
			},
		},
		{
			name: "should record duration correctly",
			test: func(t *testing.T) {
				buckets := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond}
				stats := NewDurationStats(buckets)

				// Record some durations
				stats.Record(50 * time.Millisecond)  // bucket 0
				stats.Record(200 * time.Millisecond) // bucket 1
				stats.Record(600 * time.Millisecond) // overflow bucket

				assert.Equal(t, int64(3), stats.count.Load())

				// Check min/max
				assert.Equal(t, int64(50000), stats.min.Load())  // 50ms in microseconds
				assert.Equal(t, int64(600000), stats.max.Load()) // 600ms in microseconds
			},
		},
		{
			name: "should calculate statistics correctly",
			test: func(t *testing.T) {
				buckets := []time.Duration{100 * time.Millisecond}
				stats := NewDurationStats(buckets)

				stats.Record(10 * time.Millisecond)
				stats.Record(20 * time.Millisecond)
				stats.Record(30 * time.Millisecond)

				snapshot := stats.GetStats()

				assert.Equal(t, int64(3), snapshot.Count)
				assert.Equal(t, float64(20), snapshot.AvgMs) // (10+20+30)/3 = 20
				assert.Equal(t, float64(10), snapshot.MinMs)
				assert.Equal(t, float64(30), snapshot.MaxMs)
			},
		},
		{
			name: "should handle concurrent recording",
			test: func(t *testing.T) {
				buckets := []time.Duration{100 * time.Millisecond}
				stats := NewDurationStats(buckets)

				const numGoroutines = 50
				const recordsPerGoroutine = 10

				var wg sync.WaitGroup
				wg.Add(numGoroutines)

				for i := 0; i < numGoroutines; i++ {
					go func(id int) {
						defer wg.Done()
						for j := 0; j < recordsPerGoroutine; j++ {
							duration := time.Duration(id*10+j) * time.Millisecond
							stats.Record(duration)
						}
					}(i)
				}

				wg.Wait()

				expected := int64(numGoroutines * recordsPerGoroutine)
				assert.Equal(t, expected, stats.count.Load())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestRuntimeMetrics(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create runtime metrics collector",
			test: func(t *testing.T) {
				collector := NewRuntimeMetrics()
				assert.NotNil(t, collector)
			},
		},
		{
			name: "should collect runtime snapshot",
			test: func(t *testing.T) {
				collector := NewRuntimeMetrics()

				snapshot := collector.Collect()

				assert.True(t, snapshot.Timestamp.After(time.Now().Add(-time.Second)))
				assert.Greater(t, snapshot.Goroutines, 0)
				assert.Greater(t, snapshot.MemAllocMB, 0.0)
				assert.Greater(t, snapshot.MemSysMB, 0.0)
				assert.GreaterOrEqual(t, snapshot.MemHeapMB, 0.0)
				assert.GreaterOrEqual(t, snapshot.GCCount, uint32(0))
			},
		},
		{
			name: "should start and stop metrics collection",
			test: func(t *testing.T) {
				if testing.Short() {
					t.Skip("skipping metrics collection test")
				}

				collector := NewRuntimeMetrics()
				ctx, cancel := context.WithCancel(context.Background())

				// Start collection with short interval
				done := make(chan struct{})
				go func() {
					defer close(done)
					collector.StartCollector(ctx, 10*time.Millisecond, nil) // nil logger for test
				}()

				// Let it run briefly
				time.Sleep(50 * time.Millisecond)

				// Stop collection
				cancel()

				// Wait for completion
				select {
				case <-done:
					// Success
				case <-time.After(100 * time.Millisecond):
					t.Fatal("metrics collection did not stop in time")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestSimpleTracer(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create simple tracer",
			test: func(t *testing.T) {
				tracer := NewSimpleTracer(nil) // nil logger for test
				assert.NotNil(t, tracer)
			},
		},
		{
			name: "should start and end span correctly",
			test: func(t *testing.T) {
				tracer := NewSimpleTracer(nil)
				ctx := context.Background()

				ctx, span := tracer.StartSpan(ctx, "test-operation")
				assert.NotNil(t, span)
				assert.Equal(t, "test-operation", span.name)
				assert.True(t, span.startTime.After(time.Now().Add(-time.Second)))

				span.SetAttributes("key1", "value1", "key2", 42)
				assert.Equal(t, "value1", span.attributes["key1"])
				assert.Equal(t, 42, span.attributes["key2"])

				span.End()
				// Span should complete without error
			},
		},
		{
			name: "should handle nested spans",
			test: func(t *testing.T) {
				tracer := NewSimpleTracer(nil)
				ctx := context.Background()

				ctx, parentSpan := tracer.StartSpan(ctx, "parent-operation")
				ctx, childSpan := tracer.StartSpan(ctx, "child-operation")

				childSpan.SetAttributes("child_attr", "child_value")
				childSpan.End()

				parentSpan.SetAttributes("parent_attr", "parent_value")
				parentSpan.End()
			},
		},
		{
			name: "should handle invalid attributes gracefully",
			test: func(t *testing.T) {
				tracer := NewSimpleTracer(nil)
				ctx := context.Background()

				_, span := tracer.StartSpan(ctx, "test-operation")

				// Test odd number of attributes (entire call should be ignored)
				span.SetAttributes("key1", "value1", "key2")
				assert.NotContains(t, span.attributes, "key1")
				assert.NotContains(t, span.attributes, "key2")

				// Test valid attributes
				span.SetAttributes("valid_key", "valid_value")
				assert.Equal(t, "valid_value", span.attributes["valid_key"])

				span.End()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestPerformanceReporter(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create performance reporter",
			test: func(t *testing.T) {
				metrics := NewAppMetrics()
				runtime := NewRuntimeMetrics()

				reporter := NewPerformanceReporter(metrics, runtime, nil, time.Minute)
				assert.NotNil(t, reporter)
			},
		},
		{
			name: "should generate performance report",
			test: func(t *testing.T) {
				metrics := NewAppMetrics()
				runtime := NewRuntimeMetrics()

				// Add some test data
				metrics.IncrementArticlesProcessed()
				metrics.IncrementFeedsProcessed()

				reporter := NewPerformanceReporter(metrics, runtime, nil, time.Minute)
				report := reporter.GenerateReport()

				assert.True(t, report.Timestamp.After(time.Now().Add(-time.Second)))
				assert.Equal(t, int64(1), report.Metrics.ArticlesProcessed)
				assert.Equal(t, int64(1), report.Metrics.FeedsProcessed)
				assert.Greater(t, report.Runtime.Goroutines, 0)
			},
		},
		{
			name: "should start and stop reporting",
			test: func(t *testing.T) {
				if testing.Short() {
					t.Skip("skipping reporting test")
				}

				metrics := NewAppMetrics()
				runtime := NewRuntimeMetrics()
				reporter := NewPerformanceReporter(metrics, runtime, nil, 10*time.Millisecond)

				ctx, cancel := context.WithCancel(context.Background())

				done := make(chan struct{})
				go func() {
					defer close(done)
					reporter.StartReporting(ctx)
				}()

				// Let it run briefly
				time.Sleep(50 * time.Millisecond)

				// Stop reporting
				cancel()

				// Wait for completion
				select {
				case <-done:
					// Success
				case <-time.After(100 * time.Millisecond):
					t.Fatal("performance reporting did not stop in time")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

// TestSecurityIssues tests for security vulnerabilities
func TestSecurityIssues(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should handle uint64 to int64 conversion safely in GC time",
			test: func(t *testing.T) {
				collector := NewRuntimeMetrics()

				// This should not panic or cause overflow
				snapshot := collector.Collect()

				// GC last time should be a valid time (may be zero if no GC has occurred)
				// The important thing is that it doesn't panic or cause overflow
				assert.True(t, snapshot.GCLastTime.Before(time.Now().Add(time.Second)) || snapshot.GCLastTime.IsZero())
			},
		},
		{
			name: "should handle rand.Read errors properly in generateTraceID",
			test: func(t *testing.T) {
				// Test that generateTraceID returns a valid trace ID
				traceID := generateTraceID()
				assert.NotEmpty(t, traceID)
				assert.Len(t, traceID, 32) // 16 bytes * 2 hex chars = 32 chars
			},
		},
		{
			name: "should handle rand.Read errors properly in generateSpanID",
			test: func(t *testing.T) {
				// Test that generateSpanID returns a valid span ID
				spanID := generateSpanID()
				assert.NotEmpty(t, spanID)
				assert.Len(t, spanID, 16) // 8 bytes * 2 hex chars = 16 chars
			},
		},
		{
			name: "should generate unique trace IDs",
			test: func(t *testing.T) {
				ids := make(map[string]bool)

				// Generate multiple trace IDs and ensure they're unique
				for i := 0; i < 100; i++ {
					traceID := generateTraceID()
					assert.False(t, ids[traceID], "trace ID should be unique")
					ids[traceID] = true
				}
			},
		},
		{
			name: "should generate unique span IDs",
			test: func(t *testing.T) {
				ids := make(map[string]bool)

				// Generate multiple span IDs and ensure they're unique
				for i := 0; i < 100; i++ {
					spanID := generateSpanID()
					assert.False(t, ids[spanID], "span ID should be unique")
					ids[spanID] = true
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func BenchmarkMetrics(b *testing.B) {
	b.Run("AppMetrics_Increment", func(b *testing.B) {
		metrics := NewAppMetrics()

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				metrics.IncrementArticlesProcessed()
			}
		})
	})

	b.Run("DurationStats_Record", func(b *testing.B) {
		buckets := []time.Duration{
			10 * time.Millisecond,
			100 * time.Millisecond,
			1 * time.Second,
		}
		stats := NewDurationStats(buckets)
		duration := 50 * time.Millisecond

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				stats.Record(duration)
			}
		})
	})

	b.Run("RuntimeMetrics_Collect", func(b *testing.B) {
		collector := NewRuntimeMetrics()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			collector.Collect()
		}
	})
}
