package logger

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestContextLogger_WithContext(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected []string
	}{
		{
			name:     "empty context",
			ctx:      context.Background(),
			expected: []string{},
		},
		{
			name: "context with request ID",
			ctx:  context.WithValue(context.Background(), RequestIDKey, "test-request-123"),
			expected: []string{"request_id=test-request-123"},
		},
		{
			name: "context with user ID",
			ctx:  context.WithValue(context.Background(), UserIDKey, "user-456"),
			expected: []string{"user_id=user-456"},
		},
		{
			name: "context with operation",
			ctx:  context.WithValue(context.Background(), OperationKey, "create_feed"),
			expected: []string{"operation=create_feed"},
		},
		{
			name: "context with all values",
			ctx: func() context.Context {
				ctx := context.WithValue(context.Background(), RequestIDKey, "req-123")
				ctx = context.WithValue(ctx, UserIDKey, "user-456")
				ctx = context.WithValue(ctx, OperationKey, "fetch_feed")
				return ctx
			}(),
			expected: []string{"request_id=req-123", "user_id=user-456", "operation=fetch_feed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			baseLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
			contextLogger := NewContextLogger(baseLogger)

			logger := contextLogger.WithContext(tt.ctx)
			logger.Info("test message")

			output := buf.String()
			for _, expected := range tt.expected {
				if !bytes.Contains(buf.Bytes(), []byte(expected)) {
					t.Errorf("expected output to contain %q, got %q", expected, output)
				}
			}
		})
	}
}

func TestContextLogger_LogDuration(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	contextLogger := NewContextLogger(baseLogger)

	ctx := context.WithValue(context.Background(), RequestIDKey, "test-req")
	duration := 150 * time.Millisecond
	operation := "test_operation"

	contextLogger.LogDuration(ctx, operation, duration)

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("request_id=test-req")) {
		t.Errorf("expected output to contain request_id, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("operation=test_operation")) {
		t.Errorf("expected output to contain operation name, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("duration_ms=150")) {
		t.Errorf("expected output to contain duration, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("operation completed")) {
		t.Errorf("expected output to contain completion message, got %q", output)
	}
}

func TestContextLogger_LogError(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	contextLogger := NewContextLogger(baseLogger)

	ctx := context.WithValue(context.Background(), RequestIDKey, "error-req")
	operation := "test_operation"
	testError := &TestError{msg: "test error message"}

	contextLogger.LogError(ctx, operation, testError)

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("request_id=error-req")) {
		t.Errorf("expected output to contain request_id, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("operation=test_operation")) {
		t.Errorf("expected output to contain operation name, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("operation failed")) {
		t.Errorf("expected output to contain failure message, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("test error message")) {
		t.Errorf("expected output to contain error message, got %q", output)
	}
}

func TestPerformanceLogger_MeasureOperation(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	perfLogger := NewPerformanceLogger(baseLogger)
	ctx := context.WithValue(context.Background(), RequestIDKey, "perf-test")

	// Test measuring an operation
	timer := perfLogger.StartTimer(ctx, "test_operation")
	time.Sleep(10 * time.Millisecond) // Simulate work
	timer.End()

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("request_id=perf-test")) {
		t.Errorf("expected output to contain request_id, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("operation=test_operation")) {
		t.Errorf("expected output to contain operation name, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("operation completed")) {
		t.Errorf("expected output to contain completion message, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("duration_ms")) {
		t.Errorf("expected output to contain duration, got %q", output)
	}
}

func TestPerformanceLogger_MeasureWithError(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	
	perfLogger := NewPerformanceLogger(baseLogger)
	ctx := context.WithValue(context.Background(), RequestIDKey, "error-test")

	// Test measuring an operation that fails
	timer := perfLogger.StartTimer(ctx, "failing_operation")
	time.Sleep(5 * time.Millisecond)
	timer.EndWithError(&TestError{msg: "operation failed"})

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("request_id=error-test")) {
		t.Errorf("expected output to contain request_id, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("operation=failing_operation")) {
		t.Errorf("expected output to contain operation name, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("operation failed")) {
		t.Errorf("expected output to contain error message, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("operation failed")) {
		t.Errorf("expected output to contain error text, got %q", output)
	}
}

func TestPerformanceLogger_LogSlowOperation(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	
	perfLogger := NewPerformanceLogger(baseLogger)
	ctx := context.WithValue(context.Background(), RequestIDKey, "slow-test")

	// Log a slow operation
	perfLogger.LogSlowOperation(ctx, "slow_db_query", 500*time.Millisecond, 100*time.Millisecond)

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("request_id=slow-test")) {
		t.Errorf("expected output to contain request_id, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("operation=slow_db_query")) {
		t.Errorf("expected output to contain operation name, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("slow operation detected")) {
		t.Errorf("expected output to contain slow operation warning, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("duration_ms=500")) {
		t.Errorf("expected output to contain actual duration, got %q", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("threshold_ms=100")) {
		t.Errorf("expected output to contain threshold, got %q", output)
	}
}

// Test helper for errors
type TestError struct {
	msg string
}

func (e *TestError) Error() string {
	return e.msg
}