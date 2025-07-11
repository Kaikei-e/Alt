package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoggerFactory(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create logger factory with config",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: true,
					SamplingRate:    100,
				}

				factory := NewLoggerFactory(config)
				assert.NotNil(t, factory)
				assert.Equal(t, config, factory.config)
			},
		},
		{
			name: "should return same logger instance for same component",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: false,
					SamplingRate:    1,
				}

				factory := NewLoggerFactory(config)

				logger1 := factory.GetLogger("test-component")
				logger2 := factory.GetLogger("test-component")

				assert.Equal(t, logger1, logger2)
			},
		},
		{
			name: "should return different loggers for different components",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: false,
					SamplingRate:    1,
				}

				factory := NewLoggerFactory(config)

				logger1 := factory.GetLogger("component1")
				logger2 := factory.GetLogger("component2")

				assert.NotEqual(t, logger1, logger2)
				assert.Equal(t, "component1", logger1.component)
				assert.Equal(t, "component2", logger2.component)
			},
		},
		{
			name: "should handle concurrent access safely",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: false,
					SamplingRate:    1,
				}

				factory := NewLoggerFactory(config)
				const numGoroutines = 100
				loggers := make([]*OptimizedLogger, numGoroutines)

				var wg sync.WaitGroup
				wg.Add(numGoroutines)

				for i := 0; i < numGoroutines; i++ {
					go func(index int) {
						defer wg.Done()
						loggers[index] = factory.GetLogger("concurrent-component")
					}(i)
				}

				wg.Wait()

				// All loggers should be the same instance
				for i := 1; i < numGoroutines; i++ {
					assert.Equal(t, loggers[0], loggers[i])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestSamplingLogger(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create sampling logger",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: true,
					SamplingRate:    10,
				}

				logger := NewOptimizedLogger("test", config)
				samplingLogger := NewSamplingLogger(logger, 10)

				assert.NotNil(t, samplingLogger)
				assert.Equal(t, 10, samplingLogger.samplingRate)
			},
		},
		{
			name: "should sample logs based on rate",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: true,
					SamplingRate:    3,
				}

				logger := NewOptimizedLogger("test", config)
				samplingLogger := NewSamplingLogger(logger, 3)

				// Call LogSampled multiple times
				for i := 0; i < 10; i++ {
					samplingLogger.LogSampled("info", "test message", "key", "value")
				}

				// Counter should be at 10
				assert.Equal(t, uint64(10), samplingLogger.counter)
			},
		},
		{
			name: "should handle concurrent sampling safely",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: true,
					SamplingRate:    1,
				}

				logger := NewOptimizedLogger("test", config)
				samplingLogger := NewSamplingLogger(logger, 1)

				const numGoroutines = 10
				const logsPerGoroutine = 5

				var wg sync.WaitGroup
				wg.Add(numGoroutines)

				for i := 0; i < numGoroutines; i++ {
					go func() {
						defer wg.Done()
						for j := 0; j < logsPerGoroutine; j++ {
							samplingLogger.LogSampled("info", "concurrent test", "id", j)
						}
					}()
				}

				wg.Wait()

				expected := uint64(numGoroutines * logsPerGoroutine)
				assert.Equal(t, expected, samplingLogger.counter)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestConditionalLogger(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create conditional logger",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: false,
					SamplingRate:    1,
				}

				logger := NewOptimizedLogger("test", config)
				conditions := []LogCondition{
					func(level string, msg string) bool {
						return level == "error"
					},
				}

				conditionalLogger := NewConditionalLogger(logger, conditions)
				assert.NotNil(t, conditionalLogger)
				assert.Len(t, conditionalLogger.conditions, 1)
			},
		},
		{
			name: "should evaluate conditions correctly",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: false,
					SamplingRate:    1,
				}

				logger := NewOptimizedLogger("test", config)

				// Only allow error level logs
				conditions := []LogCondition{
					func(level string, msg string) bool {
						return level == "error"
					},
				}

				conditionalLogger := NewConditionalLogger(logger, conditions)

				assert.True(t, conditionalLogger.ShouldLog("error", "error message"))
				assert.False(t, conditionalLogger.ShouldLog("info", "info message"))
				assert.False(t, conditionalLogger.ShouldLog("debug", "debug message"))
			},
		},
		{
			name: "should handle multiple conditions with AND logic",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: false,
					SamplingRate:    1,
				}

				logger := NewOptimizedLogger("test", config)

				conditions := []LogCondition{
					func(level string, msg string) bool {
						return level == "error"
					},
					func(level string, msg string) bool {
						return len(msg) > 5
					},
				}

				conditionalLogger := NewConditionalLogger(logger, conditions)

				assert.True(t, conditionalLogger.ShouldLog("error", "long error message"))
				assert.False(t, conditionalLogger.ShouldLog("error", "short"))
				assert.False(t, conditionalLogger.ShouldLog("info", "long info message"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestLogFieldCache(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create log field cache",
			test: func(t *testing.T) {
				cache := NewLogFieldCache()
				assert.NotNil(t, cache)
				assert.NotNil(t, cache.fields)
			},
		},
		{
			name: "should cache computed values",
			test: func(t *testing.T) {
				cache := NewLogFieldCache()

				computeCount := 0
				compute := func() interface{} {
					computeCount++
					return "computed_value"
				}

				// First call should compute
				value1 := cache.GetOrCompute("test_key", compute)
				assert.Equal(t, "computed_value", value1)
				assert.Equal(t, 1, computeCount)

				// Second call should use cache
				value2 := cache.GetOrCompute("test_key", compute)
				assert.Equal(t, "computed_value", value2)
				assert.Equal(t, 1, computeCount) // Should not increment
			},
		},
		{
			name: "should handle concurrent access safely",
			test: func(t *testing.T) {
				cache := NewLogFieldCache()

				const numGoroutines = 10
				computeCount := int64(0)

				var wg sync.WaitGroup
				wg.Add(numGoroutines)

				for i := 0; i < numGoroutines; i++ {
					go func() {
						defer wg.Done()
						cache.GetOrCompute("concurrent_key", func() interface{} {
							atomic.AddInt64(&computeCount, 1)
							time.Sleep(1 * time.Millisecond) // Simulate computation
							return "concurrent_value"
						})
					}()
				}

				wg.Wait()

				// Should compute only once despite concurrent access
				assert.Equal(t, int64(1), atomic.LoadInt64(&computeCount))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestOptimizedLogger(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create optimized logger with context",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: false,
					SamplingRate:    1,
				}

				logger := NewOptimizedLogger("test-component", config)
				assert.NotNil(t, logger)
				assert.Equal(t, "test-component", logger.component)
			},
		},
		{
			name: "should create logger with context",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: false,
					SamplingRate:    1,
				}

				logger := NewOptimizedLogger("test-component", config)

				ctx := context.WithValue(context.Background(), "request_id", "req-123")
				ctx = context.WithValue(ctx, "user_id", "user-456")
				ctx = context.WithValue(ctx, "trace_id", "trace-789")

				contextLogger := logger.WithContext(ctx)
				assert.NotNil(t, contextLogger)

				// Context should be embedded in the logger
				assert.Contains(t, contextLogger.contextFields, "request_id")
				assert.Contains(t, contextLogger.contextFields, "user_id")
				assert.Contains(t, contextLogger.contextFields, "trace_id")
			},
		},
		{
			name: "should log with proper format and context",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: false,
					SamplingRate:    1,
				}

				logger := NewOptimizedLogger("test-component", config)

				ctx := context.WithValue(context.Background(), "request_id", "req-123")
				contextLogger := logger.WithContext(ctx)

				// This should not panic
				contextLogger.Info("test message", "key", "value")
				contextLogger.Error("error message", "error", "test error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func BenchmarkOptimizedLogging(b *testing.B) {
	config := LogConfig{
		Level:           "info",
		Format:          "json",
		SamplingEnabled: false,
		SamplingRate:    1,
	}

	b.Run("OptimizedLogger_Info", func(b *testing.B) {
		logger := NewOptimizedLogger("benchmark", config)

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				logger.Info("benchmark message", "key", "value")
			}
		})
	})

	b.Run("SamplingLogger_LogSampled", func(b *testing.B) {
		logger := NewOptimizedLogger("benchmark", config)
		samplingLogger := NewSamplingLogger(logger, 100)

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				samplingLogger.LogSampled("info", "sampled message", "key", "value")
			}
		})
	})

	b.Run("LogFieldCache_GetOrCompute", func(b *testing.B) {
		cache := NewLogFieldCache()

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cache.GetOrCompute("test_key", func() interface{} {
					return "cached_value"
				})
			}
		})
	})
}

// TestSamplingLoggerSecurityIssues tests for security vulnerabilities in sampling logger
func TestSamplingLoggerSecurityIssues(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should handle negative sampling rate safely",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: true,
					SamplingRate:    -1, // negative value
				}

				logger := NewOptimizedLogger("test", config)

				// This should not panic or cause overflow
				samplingLogger := NewSamplingLogger(logger, -1)
				assert.NotNil(t, samplingLogger)
				assert.Equal(t, -1, samplingLogger.samplingRate)

				// LogSampled should handle negative sampling rate without panic
				samplingLogger.LogSampled("info", "test message", "key", "value")
			},
		},
		{
			name: "should handle zero sampling rate safely",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: true,
					SamplingRate:    0,
				}

				logger := NewOptimizedLogger("test", config)
				samplingLogger := NewSamplingLogger(logger, 0)

				// This should not panic or cause division by zero
				samplingLogger.LogSampled("info", "test message", "key", "value")
			},
		},
		{
			name: "should handle large sampling rate safely",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: true,
					SamplingRate:    1000000,
				}

				logger := NewOptimizedLogger("test", config)
				samplingLogger := NewSamplingLogger(logger, 1000000)

				// This should not cause overflow when converting int to uint64
				for i := 0; i < 10; i++ {
					samplingLogger.LogSampled("info", "test message", "key", "value")
				}

				assert.Equal(t, uint64(10), samplingLogger.counter)
			},
		},
		{
			name: "should handle concurrent access with large sampling rate",
			test: func(t *testing.T) {
				config := LogConfig{
					Level:           "info",
					Format:          "json",
					SamplingEnabled: true,
					SamplingRate:    1000000,
				}

				logger := NewOptimizedLogger("test", config)
				samplingLogger := NewSamplingLogger(logger, 1000000)

				const numGoroutines = 10
				const logsPerGoroutine = 5

				var wg sync.WaitGroup
				wg.Add(numGoroutines)

				for i := 0; i < numGoroutines; i++ {
					go func() {
						defer wg.Done()
						for j := 0; j < logsPerGoroutine; j++ {
							samplingLogger.LogSampled("info", "concurrent test", "id", j)
						}
					}()
				}

				wg.Wait()

				expected := uint64(numGoroutines * logsPerGoroutine)
				assert.Equal(t, expected, samplingLogger.counter)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}
