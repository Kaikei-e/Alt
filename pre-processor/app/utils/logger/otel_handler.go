package logger

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"
)

// OTelHandler is a slog.Handler that exports logs via OpenTelemetry
type OTelHandler struct {
	logger log.Logger
	attrs  []slog.Attr
	groups []string
	level  slog.Level
}

// NewOTelHandler creates a new OTelHandler
func NewOTelHandler(opts ...OTelHandlerOption) *OTelHandler {
	h := &OTelHandler{
		logger: global.GetLoggerProvider().Logger("slog-otel-bridge"),
		level:  slog.LevelInfo,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// OTelHandlerOption configures OTelHandler
type OTelHandlerOption func(*OTelHandler)

// WithOTelLevel sets the minimum log level for OTel export
func WithOTelLevel(level slog.Level) OTelHandlerOption {
	return func(h *OTelHandler) {
		h.level = level
	}
}

// Enabled reports whether the handler handles records at the given level
func (h *OTelHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle handles the Record
func (h *OTelHandler) Handle(ctx context.Context, r slog.Record) error {
	rec := log.Record{}
	rec.SetTimestamp(r.Time)
	rec.SetBody(log.StringValue(r.Message))
	rec.SetSeverity(slogLevelToOTel(r.Level))
	rec.SetSeverityText(r.Level.String())

	// Add trace context if available
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		sc := span.SpanContext()
		rec.AddAttributes(
			log.String("trace_id", sc.TraceID().String()),
			log.String("span_id", sc.SpanID().String()),
		)
	}

	// Add pre-defined attributes
	for _, attr := range h.attrs {
		rec.AddAttributes(slogAttrToOTel(h.groups, attr))
	}

	// Add record attributes
	r.Attrs(func(a slog.Attr) bool {
		rec.AddAttributes(slogAttrToOTel(h.groups, a))
		return true
	})

	h.logger.Emit(ctx, rec)
	return nil
}

// WithAttrs returns a new Handler with the given attributes added
func (h *OTelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &OTelHandler{
		logger: h.logger,
		attrs:  newAttrs,
		groups: h.groups,
		level:  h.level,
	}
}

// WithGroup returns a new Handler with the given group appended
func (h *OTelHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name
	return &OTelHandler{
		logger: h.logger,
		attrs:  h.attrs,
		groups: newGroups,
		level:  h.level,
	}
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
	if len(groups) > 0 {
		for _, g := range groups {
			key = g + "." + key
		}
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
	case slog.KindDuration:
		return log.Int64(key, int64(a.Value.Duration()))
	case slog.KindTime:
		return log.Int64(key, a.Value.Time().UnixNano())
	case slog.KindGroup:
		return log.String(key, a.Value.String())
	default:
		return log.String(key, a.Value.String())
	}
}

// MultiHandler sends logs to multiple handlers
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a handler that writes to both stdout and OTel
func NewMultiHandler(stdoutHandler slog.Handler, level slog.Level) *MultiHandler {
	return &MultiHandler{
		handlers: []slog.Handler{
			stdoutHandler,
			NewOTelHandler(WithOTelLevel(level)),
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
