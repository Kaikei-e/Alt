package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func captureRecords(t *testing.T, fn func(h slog.Handler)) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	fn(base)
	records := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		records = append(records, m)
	}
	return records
}

func TestFilteringHandler_DropsLoopbackEOF(t *testing.T) {
	records := captureRecords(t, func(base slog.Handler) {
		h := newFilteringHandler(base)
		logger := slog.New(h)
		logger.Warn("http: TLS handshake error from 127.0.0.1:58796: EOF")
		logger.Warn("http: TLS handshake error from 127.0.0.1:44492: EOF")
	})
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d: %v", len(records), records)
	}
}

func TestFilteringHandler_PassesThroughRealTLSErrors(t *testing.T) {
	cases := []string{
		"http: TLS handshake error from 10.0.0.5:49152: remote error: tls: bad certificate",
		"http: TLS handshake error from 127.0.0.1:49152: read tcp 127.0.0.1:9443->127.0.0.1:49152: i/o timeout",
		"http: TLS handshake error from 127.0.0.1:49152: tls: client didn't provide a certificate",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			records := captureRecords(t, func(base slog.Handler) {
				logger := slog.New(newFilteringHandler(base))
				logger.Warn(msg)
			})
			if len(records) != 1 {
				t.Fatalf("expected 1 record, got %d: %v", len(records), records)
			}
			if records[0]["msg"] != msg {
				t.Fatalf("msg=%v want %q", records[0]["msg"], msg)
			}
		})
	}
}

func TestFilteringHandler_PassesThroughUnrelatedMessages(t *testing.T) {
	records := captureRecords(t, func(base slog.Handler) {
		logger := slog.New(newFilteringHandler(base))
		logger.Info("tick ok", "state", "fresh")
	})
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0]["msg"] != "tick ok" {
		t.Fatalf("msg=%v", records[0]["msg"])
	}
}

func TestFilteringHandler_WithAttrsAndGroupPreserveFiltering(t *testing.T) {
	records := captureRecords(t, func(base slog.Handler) {
		h := newFilteringHandler(base).WithAttrs([]slog.Attr{slog.String("service", "proxy")})
		logger := slog.New(h.WithGroup("tls"))
		logger.Warn("http: TLS handshake error from 127.0.0.1:1: EOF")
		logger.Warn("http: TLS handshake error from 10.0.0.5:1: tls: bad certificate")
	})
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d: %v", len(records), records)
	}
}

func TestFilteringHandler_EnabledRespectsBase(t *testing.T) {
	base := slog.NewJSONHandler(nil, &slog.HandlerOptions{Level: slog.LevelError})
	h := newFilteringHandler(base)
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatalf("expected base Level=Error to disable Info")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Fatalf("expected base Level=Error to allow Error")
	}
}

func TestNewProxyErrorLog_WritesThroughSlog(t *testing.T) {
	records := captureRecords(t, func(base slog.Handler) {
		prev := slog.Default()
		slog.SetDefault(slog.New(base))
		defer slog.SetDefault(prev)

		logger := newProxyErrorLog()
		logger.Print("http: TLS handshake error from 127.0.0.1:9999: EOF")
		logger.Print("http: TLS handshake error from 10.0.0.1:80: tls: bad certificate")
	})
	if len(records) != 1 {
		t.Fatalf("expected 1 record (real error passed), got %d: %v", len(records), records)
	}
	if !strings.Contains(records[0]["msg"].(string), "bad certificate") {
		t.Fatalf("wrong msg survived: %v", records[0]["msg"])
	}
	if records[0]["level"] != "WARN" {
		t.Fatalf("expected WARN level, got %v", records[0]["level"])
	}
}
