package otel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	plogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	ptrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestProvider_InitAndExport(t *testing.T) {
	originalTracer := otel.GetTracerProvider()
	originalLogger := global.GetLoggerProvider()
	defer func() {
		otel.SetTracerProvider(originalTracer)
		global.SetLoggerProvider(originalLogger)
	}()

	var traceReqCount, logReqCount int32
	var traceAuth, logAuth string
	var mu sync.Mutex
	var lastTraceBody, lastLogBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()

		if r.URL.Path == "/v1/traces" {
			atomic.AddInt32(&traceReqCount, 1)
			traceAuth = r.Header.Get("Authorization")
			lastTraceBody = body
		} else if r.URL.Path == "/v1/logs" {
			atomic.AddInt32(&logReqCount, 1)
			logAuth = r.Header.Get("Authorization")
			lastLogBody = body
		} else {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		if r.Header.Get("Content-Type") != "application/x-protobuf" {
			t.Errorf("Expected protobuf content type, got %s", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	tokenContent := "valid-token-123="
	tmpFile, err := os.CreateTemp("", "token-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Write([]byte(tokenContent))
	tmpFile.Close()

	os.Setenv("RASK_INGEST_TOKEN_FILE", tmpFile.Name())
	os.Setenv("RASK_INGEST_TOKEN", tokenContent)
	os.Setenv("OTEL_TRACE_SAMPLE_RATIO", "1.0")

	cfg := ConfigFromEnv()
	cfg.Enabled = true
	cfg.OTLPEndpoint = ts.URL
	cfg.IngestToken = tokenContent

	ctx := context.Background()
	shutdown, err := InitProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("InitProvider failed: %v", err)
	}
	defer func() {
		ctxShut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctxShut)
	}()

	tracer := otel.Tracer("test-tracer")
	_, span := tracer.Start(ctx, "test-span")
	span.End()

	logger := global.GetLoggerProvider().Logger("test-logger")
	var rec log.Record
	logger.Emit(ctx, rec)

	ctxFlush, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer flushCancel()

	if tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		tp.ForceFlush(ctxFlush)
	} else {
		t.Errorf("TracerProvider is not sdktrace.TracerProvider")
	}

	if lp, ok := global.GetLoggerProvider().(*sdklog.LoggerProvider); ok {
		lp.ForceFlush(ctxFlush)
	} else {
		t.Errorf("LoggerProvider is not sdklog.LoggerProvider")
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if traceReqCount == 0 {
		t.Errorf("Expected trace requests, got 0")
	}
	if logReqCount == 0 {
		t.Errorf("Expected log requests, got 0")
	}

	if traceAuth != "Bearer "+tokenContent {
		t.Errorf("Trace Auth mismatch: got %s", traceAuth)
	}
	if logAuth != "Bearer "+tokenContent {
		t.Errorf("Log Auth mismatch: got %s", logAuth)
	}

	var traceReq ptrace.ExportTraceServiceRequest
	if err := proto.Unmarshal(lastTraceBody, &traceReq); err != nil {
		t.Fatalf("Failed to decode trace req: %v", err)
	}
	if len(traceReq.ResourceSpans) == 0 {
		t.Errorf("Expected non-empty ResourceSpans")
	} else if len(traceReq.ResourceSpans[0].ScopeSpans) == 0 {
		t.Errorf("Expected non-empty ScopeSpans")
	} else if len(traceReq.ResourceSpans[0].ScopeSpans[0].Spans) == 0 {
		t.Errorf("Expected non-empty Spans")
	}

	var logReq plogs.ExportLogsServiceRequest
	if err := proto.Unmarshal(lastLogBody, &logReq); err != nil {
		t.Fatalf("Failed to decode log req: %v", err)
	}
	if len(logReq.ResourceLogs) == 0 {
		t.Errorf("Expected non-empty ResourceLogs")
	} else if len(logReq.ResourceLogs[0].ScopeLogs) == 0 {
		t.Errorf("Expected non-empty ScopeLogs")
	} else if len(logReq.ResourceLogs[0].ScopeLogs[0].LogRecords) == 0 {
		t.Errorf("Expected non-empty LogRecords")
	}
}

func TestProvider_RedirectFail(t *testing.T) {
	originalTracer := otel.GetTracerProvider()
	originalLogger := global.GetLoggerProvider()
	defer func() {
		otel.SetTracerProvider(originalTracer)
		global.SetLoggerProvider(originalLogger)
	}()

	var reqCount int32
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqCount++
		mu.Unlock()

		if r.Header.Get("Authorization") == "" {
			t.Errorf("Missing authorization header on redirect test")
		}

		http.Redirect(w, r, "/dev/null", http.StatusFound)
	}))
	defer ts.Close()

	os.Setenv("OTEL_TRACE_SAMPLE_RATIO", "1.0")
	cfg := ConfigFromEnv()
	cfg.Enabled = true
	cfg.OTLPEndpoint = ts.URL
	cfg.IngestToken = "valid-token-123="

	ctx := context.Background()
	shutdown, err := InitProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("InitProvider failed: %v", err)
	}
	defer func() {
		ctxShut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctxShut)
	}()

	tracer := otel.Tracer("test-tracer")
	_, span := tracer.Start(ctx, "test-span")
	span.End()

	ctxFlush, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer flushCancel()

	if tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		err := tp.ForceFlush(ctxFlush)
		if err == nil {
			t.Errorf("Expected ForceFlush to fail due to redirect policy, but it succeeded")
		}
	}

	mu.Lock()
	if reqCount == 0 {
		t.Errorf("Expected at least 1 request to be made before redirect")
	}
	mu.Unlock()
}

func TestProvider_InvalidToken(t *testing.T) {
	cfg := ConfigFromEnv()
	cfg.Enabled = true
	cfg.OTLPEndpoint = "http://localhost:4318"
	cfg.IngestToken = "invalid token!@#" // invalid format

	ctx := context.Background()
	_, err := InitProvider(ctx, cfg)
	if err == nil {
		t.Fatalf("Expected InitProvider to fail with invalid token")
	}
}
