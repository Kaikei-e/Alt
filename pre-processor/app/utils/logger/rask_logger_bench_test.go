// ABOUTME: This file provides benchmark tests for Rask logger performance comparison
// ABOUTME: Ensures Rask logger performance doesn't degrade compared to existing logger
package logger

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func BenchmarkRaskLogger_BasicThroughput(b *testing.B) {
	var buf bytes.Buffer
	raskLogger := NewRaskLogger(&buf, "pre-processor")
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			raskLogger.Info("benchmark test message", "operation", "benchmark", "count", 42)
		}
	})
	b.StopTimer()
}

func BenchmarkContextLogger_BasicThroughput(b *testing.B) {
	var buf bytes.Buffer
	contextLogger := NewContextLogger(&buf, "json", "info")
	ctx := context.Background()
	logger := contextLogger.WithContext(ctx)
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("benchmark test message", "operation", "benchmark", "count", 42)
		}
	})
	b.StopTimer()
}

func BenchmarkRaskLogger_WithAttributes(b *testing.B) {
	var buf bytes.Buffer
	raskLogger := NewRaskLogger(&buf, "pre-processor")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enrichedLogger := raskLogger.With(
			"request_id", "req-123",
			"trace_id", "trace-456", 
			"operation", "benchmark_test",
			"user_id", 789,
		)
		enrichedLogger.Info("message with attributes")
	}
	b.StopTimer()
}

func BenchmarkContextLogger_WithAttributes(b *testing.B) {
	var buf bytes.Buffer
	contextLogger := NewContextLogger(&buf, "json", "info")
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger := contextLogger.WithContext(ctx)
		enrichedLogger := logger.With(
			"request_id", "req-123",
			"trace_id", "trace-456",
			"operation", "benchmark_test", 
			"user_id", 789,
		)
		enrichedLogger.Info("message with attributes")
	}
	b.StopTimer()
}

func BenchmarkIntegratedLogger_RaskMode(b *testing.B) {
	var buf bytes.Buffer
	config := &LoggerConfig{
		Level:       "info",
		Format:      "json",
		ServiceName: "pre-processor",
		UseRask:     true,
	}
	
	contextLogger := NewContextLoggerWithConfig(config, &buf)
	ctx := context.Background()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger := contextLogger.WithContext(ctx)
			logger.Info("integrated benchmark message", "benchmark", true)
		}
	})
	b.StopTimer()
}

func BenchmarkIntegratedLogger_ExistingMode(b *testing.B) {
	var buf bytes.Buffer
	config := &LoggerConfig{
		Level:       "info", 
		Format:      "json",
		ServiceName: "pre-processor",
		UseRask:     false,
	}
	
	contextLogger := NewContextLoggerWithConfig(config, &buf)
	ctx := context.Background()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger := contextLogger.WithContext(ctx)
			logger.Info("integrated benchmark message", "benchmark", true)
		}
	})
	b.StopTimer()
}

func BenchmarkRaskLogger_HighVolumeLogging(b *testing.B) {
	// Discard output to focus on processing performance
	raskLogger := NewRaskLogger(io.Discard, "pre-processor")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raskLogger.Info("high volume test",
			"iteration", i,
			"module", "benchmark",
			"level", "production",
			"timestamp", "2025-07-02T11:00:00Z",
			"processing_time_ms", 15,
		)
	}
	b.StopTimer()
}

func BenchmarkContextLogger_HighVolumeLogging(b *testing.B) {
	// Discard output to focus on processing performance  
	contextLogger := NewContextLogger(io.Discard, "json", "info")
	ctx := context.Background()
	logger := contextLogger.WithContext(ctx)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("high volume test",
			"iteration", i,
			"module", "benchmark", 
			"level", "production",
			"timestamp", "2025-07-02T11:00:00Z",
			"processing_time_ms", 15,
		)
	}
	b.StopTimer()
}

func BenchmarkRaskLogger_ErrorLogging(b *testing.B) {
	var buf bytes.Buffer
	raskLogger := NewRaskLogger(&buf, "pre-processor")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raskLogger.Error("benchmark error occurred",
			"error_code", "BENCH_001",
			"retry_count", 3,
			"operation", "benchmark_test",
		)
	}
	b.StopTimer()
}

func BenchmarkContextLogger_ErrorLogging(b *testing.B) {
	var buf bytes.Buffer
	contextLogger := NewContextLogger(&buf, "json", "info")
	ctx := context.Background()
	logger := contextLogger.WithContext(ctx)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Error("benchmark error occurred",
			"error_code", "BENCH_001",
			"retry_count", 3,
			"operation", "benchmark_test",
		)
	}
	b.StopTimer()
}