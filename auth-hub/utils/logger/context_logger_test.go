package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestContextLogger_WithContext_BusinessKeys(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)
	cl := NewContextLogger(logger)

	ctx := context.Background()
	ctx = WithArticleID(ctx, "article-123")
	ctx = WithFeedID(ctx, "feed-456")
	ctx = WithJobID(ctx, "job-789")
	ctx = WithProcessingStage(ctx, "validation")
	ctx = WithAIPipeline(ctx, "auth-hub")

	cl.WithContext(ctx).Info("test message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"alt.article.id", "article-123"},
		{"alt.feed.id", "feed-456"},
		{"alt.job.id", "job-789"},
		{"alt.processing.stage", "validation"},
		{"alt.ai.pipeline", "auth-hub"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := logEntry[tt.key]
			if !ok {
				t.Errorf("expected key %q to be present in log", tt.key)
				return
			}
			if got != tt.expected {
				t.Errorf("expected %q to be %q, got %q", tt.key, tt.expected, got)
			}
		})
	}
}

func TestContextLogger_WithContext_PartialKeys(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)
	cl := NewContextLogger(logger)

	ctx := context.Background()
	ctx = WithUserID(ctx, "user-only")

	cl.WithContext(ctx).Info("test message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if got, ok := logEntry["user_id"]; !ok || got != "user-only" {
		t.Errorf("expected user_id to be 'user-only', got %v", got)
	}

	// Other keys should not be present
	for _, key := range []string{"alt.feed.id", "alt.job.id", "alt.processing.stage", "alt.ai.pipeline"} {
		if _, ok := logEntry[key]; ok {
			t.Errorf("expected key %q to not be present in log", key)
		}
	}
}

func TestContextLogger_LogDuration(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)
	cl := NewContextLogger(logger)

	ctx := context.Background()
	ctx = WithUserID(ctx, "user-timing")

	cl.LogDuration(ctx, "session_validate", 25)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if got := logEntry["operation"]; got != "session_validate" {
		t.Errorf("expected operation to be 'session_validate', got %v", got)
	}
	if got := logEntry["duration_ms"]; got != float64(25) {
		t.Errorf("expected duration_ms to be 25, got %v", got)
	}
	if got := logEntry["user_id"]; got != "user-timing" {
		t.Errorf("expected user_id to be 'user-timing', got %v", got)
	}
}

func TestContextLogger_LogError(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)
	cl := NewContextLogger(logger)

	ctx := context.Background()
	ctx = WithUserID(ctx, "user-error")

	testErr := &testError{msg: "validation error"}
	cl.LogError(ctx, "session_validate_failed", testErr)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if got := logEntry["operation"]; got != "session_validate_failed" {
		t.Errorf("expected operation to be 'session_validate_failed', got %v", got)
	}
	if got := logEntry["user_id"]; got != "user-error" {
		t.Errorf("expected user_id to be 'user-error', got %v", got)
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestWithUserID(t *testing.T) {
	ctx := context.Background()
	ctx = WithUserID(ctx, "test-user")

	got := ctx.Value(UserIDKey)
	if got != "test-user" {
		t.Errorf("expected 'test-user', got %v", got)
	}
}

func TestWithRequestID(t *testing.T) {
	ctx := context.Background()
	ctx = WithRequestID(ctx, "test-request")

	got := ctx.Value(RequestIDKey)
	if got != "test-request" {
		t.Errorf("expected 'test-request', got %v", got)
	}
}

func TestWithArticleID(t *testing.T) {
	ctx := context.Background()
	ctx = WithArticleID(ctx, "test-article")

	got := ctx.Value(ArticleIDKey)
	if got != "test-article" {
		t.Errorf("expected 'test-article', got %v", got)
	}
}

func TestWithFeedID(t *testing.T) {
	ctx := context.Background()
	ctx = WithFeedID(ctx, "test-feed")

	got := ctx.Value(FeedIDKey)
	if got != "test-feed" {
		t.Errorf("expected 'test-feed', got %v", got)
	}
}

func TestWithJobID(t *testing.T) {
	ctx := context.Background()
	ctx = WithJobID(ctx, "test-job")

	got := ctx.Value(JobIDKey)
	if got != "test-job" {
		t.Errorf("expected 'test-job', got %v", got)
	}
}

func TestWithProcessingStage(t *testing.T) {
	ctx := context.Background()
	ctx = WithProcessingStage(ctx, "test-stage")

	got := ctx.Value(ProcessingStageKey)
	if got != "test-stage" {
		t.Errorf("expected 'test-stage', got %v", got)
	}
}

func TestWithAIPipeline(t *testing.T) {
	ctx := context.Background()
	ctx = WithAIPipeline(ctx, "test-pipeline")

	got := ctx.Value(AIPipelineKey)
	if got != "test-pipeline" {
		t.Errorf("expected 'test-pipeline', got %v", got)
	}
}
