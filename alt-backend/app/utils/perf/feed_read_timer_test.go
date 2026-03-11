package perf

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupTestTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		tp.Shutdown(context.Background())
	})
	return exporter
}

func TestNewFeedReadTimer(t *testing.T) {
	timer := NewFeedReadTimer("GetUnreadFeeds")
	if timer == nil {
		t.Fatal("expected non-nil timer")
	}
	if timer.timings.Endpoint != "GetUnreadFeeds" {
		t.Errorf("expected endpoint GetUnreadFeeds, got %s", timer.timings.Endpoint)
	}
}

func TestFeedReadTimer_StartPhase_RecordsDuration(t *testing.T) {
	exporter := setupTestTracer(t)
	timer := NewFeedReadTimer("GetUnreadFeeds")
	ctx := context.Background()

	stop := timer.StartPhase(ctx, "usecase")
	time.Sleep(10 * time.Millisecond)
	stop()

	if timer.timings.UsecaseMs < 10 {
		t.Errorf("expected usecase_ms >= 10, got %d", timer.timings.UsecaseMs)
	}

	spans := exporter.GetSpans()
	found := false
	for _, s := range spans {
		if s.Name == "perf.usecase" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected OTel span named 'perf.usecase'")
	}
}

func TestFeedReadTimer_MultiplePhasesRecordSeparately(t *testing.T) {
	setupTestTracer(t)
	timer := NewFeedReadTimer("GetAllFeeds")
	ctx := context.Background()

	stopUsecase := timer.StartPhase(ctx, "usecase")
	time.Sleep(10 * time.Millisecond)
	stopUsecase()

	stopMarshal := timer.StartPhase(ctx, "marshal")
	time.Sleep(5 * time.Millisecond)
	stopMarshal()

	if timer.timings.UsecaseMs < 10 {
		t.Errorf("expected usecase_ms >= 10, got %d", timer.timings.UsecaseMs)
	}
	if timer.timings.MarshalMs < 5 {
		t.Errorf("expected marshal_ms >= 5, got %d", timer.timings.MarshalMs)
	}
}

func TestFeedReadTimer_SetRowCount(t *testing.T) {
	timer := NewFeedReadTimer("GetUnreadFeeds")
	timer.SetRowCount(42)
	if timer.timings.RowCount != 42 {
		t.Errorf("expected row_count 42, got %d", timer.timings.RowCount)
	}
}

func TestFeedReadTimer_Log_EmitsStructuredFields(t *testing.T) {
	setupTestTracer(t)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	timer := NewFeedReadTimerWithLogger("GetUnreadFeeds", logger)
	ctx := context.Background()

	stopUsecase := timer.StartPhase(ctx, "usecase")
	stopUsecase()

	stopMarshal := timer.StartPhase(ctx, "marshal")
	stopMarshal()

	timer.SetRowCount(20)
	timer.Log(ctx)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output: %v\nbuf: %s", err, buf.String())
	}

	expectedFields := []string{"endpoint", "usecase_ms", "marshal_ms", "total_ms", "row_count", "cache_ms", "cache_hit"}
	for _, field := range expectedFields {
		if _, ok := logEntry[field]; !ok {
			t.Errorf("expected field %q in log output, got: %v", field, logEntry)
		}
	}

	if logEntry["endpoint"] != "GetUnreadFeeds" {
		t.Errorf("expected endpoint=GetUnreadFeeds, got %v", logEntry["endpoint"])
	}
	if logEntry["cache_hit"] != false {
		t.Errorf("expected cache_hit=false, got %v", logEntry["cache_hit"])
	}
}

func TestFeedReadTimer_Disabled_ReturnsNoOp(t *testing.T) {
	exporter := setupTestTracer(t)

	os.Setenv("FEED_READ_PERF_ENABLED", "false")
	defer os.Unsetenv("FEED_READ_PERF_ENABLED")

	timer := NewFeedReadTimer("GetUnreadFeeds")
	ctx := context.Background()

	stop := timer.StartPhase(ctx, "usecase")
	time.Sleep(5 * time.Millisecond)
	stop()

	// Disabled timer should not record timings
	if timer.timings.UsecaseMs != 0 {
		t.Errorf("expected usecase_ms=0 when disabled, got %d", timer.timings.UsecaseMs)
	}

	// Disabled timer should not create spans
	spans := exporter.GetSpans()
	for _, s := range spans {
		if s.Name == "perf.usecase" {
			t.Error("expected no OTel spans when disabled")
		}
	}
}

func TestFeedReadTimer_TotalMs_CoversFull(t *testing.T) {
	setupTestTracer(t)
	timer := NewFeedReadTimer("GetUnreadFeeds")
	ctx := context.Background()

	stopUsecase := timer.StartPhase(ctx, "usecase")
	time.Sleep(10 * time.Millisecond)
	stopUsecase()

	timer.Log(ctx)

	if timer.timings.TotalMs < 10 {
		t.Errorf("expected total_ms >= 10, got %d", timer.timings.TotalMs)
	}
}

func TestFeedReadTimer_CacheFieldsDefaultToZero(t *testing.T) {
	timer := NewFeedReadTimer("GetUnreadFeeds")
	if timer.timings.CacheMs != 0 {
		t.Errorf("expected cache_ms=0, got %d", timer.timings.CacheMs)
	}
	if timer.timings.CacheHit != false {
		t.Errorf("expected cache_hit=false, got %v", timer.timings.CacheHit)
	}
}
