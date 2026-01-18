package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"
)

// Init initializes a JSON logger with optional OTel support
func Init(enableOTel bool) *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))

	var handler slog.Handler
	if enableOTel {
		handler = NewMultiHandler(level)
	} else {
		jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
		// Wrap with TraceContextHandler even without OTel for trace_id/span_id in stdout
		handler = NewTraceContextHandler(jsonHandler)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Initialize ContextLogger for ADR 98/99 business context support
	GlobalContext = NewContextLogger(logger)

	return logger
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// OTelHandler is a slog.Handler that exports logs via OpenTelemetry
type OTelHandler struct {
	logger log.Logger
	attrs  []slog.Attr
	groups []string
	level  slog.Level
}

func NewOTelHandler(level slog.Level) *OTelHandler {
	return &OTelHandler{
		logger: global.GetLoggerProvider().Logger("slog-otel-bridge"),
		level:  level,
	}
}

func (h *OTelHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *OTelHandler) Handle(ctx context.Context, r slog.Record) error {
	rec := log.Record{}
	rec.SetTimestamp(r.Time)
	rec.SetBody(log.StringValue(r.Message))
	rec.SetSeverity(slogLevelToOTel(r.Level))
	rec.SetSeverityText(r.Level.String())

	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		sc := span.SpanContext()
		rec.AddAttributes(
			log.String("trace_id", sc.TraceID().String()),
			log.String("span_id", sc.SpanID().String()),
		)
	}

	for _, attr := range h.attrs {
		rec.AddAttributes(slogAttrToOTel(h.groups, attr))
	}

	r.Attrs(func(a slog.Attr) bool {
		rec.AddAttributes(slogAttrToOTel(h.groups, a))
		return true
	})

	h.logger.Emit(ctx, rec)
	return nil
}

func (h *OTelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &OTelHandler{logger: h.logger, attrs: newAttrs, groups: h.groups, level: h.level}
}

func (h *OTelHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name
	return &OTelHandler{logger: h.logger, attrs: h.attrs, groups: newGroups, level: h.level}
}

func slogLevelToOTel(level slog.Level) log.Severity {
	switch {
	case level >= slog.LevelError:
		return log.SeverityError
	case level >= slog.LevelWarn:
		return log.SeverityWarn
	case level >= slog.LevelInfo:
		return log.SeverityInfo
	default:
		return log.SeverityDebug
	}
}

func slogAttrToOTel(groups []string, a slog.Attr) log.KeyValue {
	key := a.Key
	for _, g := range groups {
		key = g + "." + key
	}

	switch a.Value.Kind() {
	case slog.KindString:
		return log.String(key, a.Value.String())
	case slog.KindInt64:
		return log.Int64(key, a.Value.Int64())
	case slog.KindFloat64:
		return log.Float64(key, a.Value.Float64())
	case slog.KindBool:
		return log.Bool(key, a.Value.Bool())
	default:
		return log.String(key, a.Value.String())
	}
}

// MultiHandler sends logs to multiple handlers
type MultiHandler struct {
	handlers []slog.Handler
}

func NewMultiHandler(level slog.Level) *MultiHandler {
	// Wrap JSONHandler with TraceContextHandler to add trace_id/span_id to stdout logs
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})

	return &MultiHandler{
		handlers: []slog.Handler{
			NewTraceContextHandler(jsonHandler),
			NewOTelHandler(level),
		},
	}
}

func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			_ = handler.Handle(ctx, r)
		}
	}
	return nil
}

func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: newHandlers}
}

func (h *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &MultiHandler{handlers: newHandlers}
}
