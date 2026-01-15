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
	ctx = WithProcessingStage(ctx, "indexing")
	ctx = WithAIPipeline(ctx, "search-indexer")

	cl.WithContext(ctx).Info("test message")

	var logEntry map[string]interface{}
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
		{"alt.processing.stage", "indexing"},
		{"alt.ai.pipeline", "search-indexer"},
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
	ctx = WithArticleID(ctx, "article-only")

	cl.WithContext(ctx).Info("test message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if got, ok := logEntry["alt.article.id"]; !ok || got != "article-only" {
		t.Errorf("expected alt.article.id to be 'article-only', got %v", got)
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
	ctx = WithArticleID(ctx, "article-timing")

	cl.LogDuration(ctx, "index_batch", 1500)

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if got := logEntry["operation"]; got != "index_batch" {
		t.Errorf("expected operation to be 'index_batch', got %v", got)
	}
	if got := logEntry["duration_ms"]; got != float64(1500) {
		t.Errorf("expected duration_ms to be 1500, got %v", got)
	}
	if got := logEntry["alt.article.id"]; got != "article-timing" {
		t.Errorf("expected alt.article.id to be 'article-timing', got %v", got)
	}
}

func TestContextLogger_LogError(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)
	cl := NewContextLogger(logger)

	ctx := context.Background()
	ctx = WithArticleID(ctx, "article-error")

	testErr := &testError{msg: "test error"}
	cl.LogError(ctx, "index_failed", testErr)

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v", err)
	}

	if got := logEntry["operation"]; got != "index_failed" {
		t.Errorf("expected operation to be 'index_failed', got %v", got)
	}
	if got := logEntry["alt.article.id"]; got != "article-error" {
		t.Errorf("expected alt.article.id to be 'article-error', got %v", got)
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
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
