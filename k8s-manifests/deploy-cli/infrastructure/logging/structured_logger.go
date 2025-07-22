// Phase R4: 構造化ログ - 構造化ログ・メタデータ・フィルタリング
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

// LogLevel represents log levels
type LogLevel slog.Level

const (
	DebugLevel LogLevel = LogLevel(slog.LevelDebug)
	InfoLevel  LogLevel = LogLevel(slog.LevelInfo)
	WarnLevel  LogLevel = LogLevel(slog.LevelWarn)
	ErrorLevel LogLevel = LogLevel(slog.LevelError)
)

// LogFormat represents log output formats
type LogFormat string

const (
	JSONFormat LogFormat = "json"
	TextFormat LogFormat = "text"
)

// StructuredLogger provides enhanced structured logging capabilities
type StructuredLogger struct {
	*slog.Logger
	config     *LoggerConfig
	filters    []LogFilter
	enrichers  []LogEnricher
	handlers   []LogHandler
	metadata   map[string]interface{}
}

// LoggerConfig holds logger configuration
type LoggerConfig struct {
	Level               LogLevel
	Format              LogFormat
	Output              *os.File
	EnableColors        bool
	EnableTimestamp     bool
	EnableCaller        bool
	EnableStackTrace    bool
	StructuredMetadata  bool
	MaxFieldLength      int
	TimestampFormat     string
}

// LogFilter filters log entries
type LogFilter interface {
	ShouldLog(level LogLevel, message string, attrs []slog.Attr) bool
}

// LogEnricher enriches log entries with additional metadata
type LogEnricher interface {
	Enrich(ctx context.Context, record *LogRecord) error
}

// LogHandler handles log entries after they're processed
type LogHandler interface {
	Handle(record *LogRecord) error
}

// LogRecord represents a complete log record
type LogRecord struct {
	Level     LogLevel
	Message   string
	Timestamp time.Time
	Attrs     map[string]interface{}
	Context   context.Context
	Caller    *CallerInfo
}

// CallerInfo holds information about the caller
type CallerInfo struct {
	Function string
	File     string
	Line     int
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(config *LoggerConfig) *StructuredLogger {
	if config == nil {
		config = DefaultLoggerConfig()
	}

	// Create slog handler based on format
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: slog.Level(config.Level),
	}

	if config.EnableCaller {
		opts.AddSource = true
	}

	switch config.Format {
	case JSONFormat:
		handler = slog.NewJSONHandler(config.Output, opts)
	case TextFormat:
		handler = slog.NewTextHandler(config.Output, opts)
	default:
		handler = slog.NewTextHandler(config.Output, opts)
	}

	logger := slog.New(handler)

	return &StructuredLogger{
		Logger:   logger,
		config:   config,
		metadata: make(map[string]interface{}),
	}
}

// DefaultLoggerConfig returns default logger configuration
func DefaultLoggerConfig() *LoggerConfig {
	return &LoggerConfig{
		Level:               InfoLevel,
		Format:              TextFormat,
		Output:              os.Stdout,
		EnableColors:        true,
		EnableTimestamp:     true,
		EnableCaller:        false,
		EnableStackTrace:    false,
		StructuredMetadata:  false,
		MaxFieldLength:      1000,
		TimestampFormat:     time.RFC3339,
	}
}

// WithContext creates a new logger with context-specific metadata
func (l *StructuredLogger) WithContext(ctx context.Context) *StructuredLogger {
	newLogger := &StructuredLogger{
		Logger:   l.Logger,
		config:   l.config,
		filters:  l.filters,
		enrichers: l.enrichers,
		handlers: l.handlers,
		metadata: make(map[string]interface{}),
	}

	// Copy existing metadata
	for k, v := range l.metadata {
		newLogger.metadata[k] = v
	}

	// Extract context values
	if requestID := ctx.Value("request_id"); requestID != nil {
		newLogger.metadata["request_id"] = requestID
	}
	if userID := ctx.Value("user_id"); userID != nil {
		newLogger.metadata["user_id"] = userID
	}
	if traceID := ctx.Value("trace_id"); traceID != nil {
		newLogger.metadata["trace_id"] = traceID
	}

	return newLogger
}

// WithFields creates a new logger with additional fields
func (l *StructuredLogger) WithFields(fields map[string]interface{}) *StructuredLogger {
	newLogger := &StructuredLogger{
		Logger:   l.Logger,
		config:   l.config,
		filters:  l.filters,
		enrichers: l.enrichers,
		handlers: l.handlers,
		metadata: make(map[string]interface{}),
	}

	// Copy existing metadata
	for k, v := range l.metadata {
		newLogger.metadata[k] = v
	}

	// Add new fields
	for k, v := range fields {
		newLogger.metadata[k] = v
	}

	return newLogger
}

// WithField creates a new logger with an additional field
func (l *StructuredLogger) WithField(key string, value interface{}) *StructuredLogger {
	return l.WithFields(map[string]interface{}{key: value})
}

// LogWithLevel logs a message at the specified level
func (l *StructuredLogger) LogWithLevel(ctx context.Context, level LogLevel, message string, attrs ...interface{}) {
	if !l.shouldLog(level) {
		return
	}

	record := &LogRecord{
		Level:     level,
		Message:   message,
		Timestamp: time.Now(),
		Attrs:     l.buildAttrs(attrs...),
		Context:   ctx,
	}

	// Add caller info if enabled
	if l.config.EnableCaller {
		record.Caller = l.getCaller(2)
	}

	// Apply filters
	if !l.passesFilters(record) {
		return
	}

	// Apply enrichers
	for _, enricher := range l.enrichers {
		if err := enricher.Enrich(ctx, record); err != nil {
			// Log enricher error but continue
			l.Logger.Error("Log enricher failed", "error", err)
		}
	}

	// Convert to slog attrs
	slogAttrs := l.convertToSlogAttrs(record)

	// Log using underlying slog logger
	l.Logger.LogAttrs(ctx, slog.Level(level), message, slogAttrs...)

	// Apply handlers
	for _, handler := range l.handlers {
		if err := handler.Handle(record); err != nil {
			// Log handler error but continue
			l.Logger.Error("Log handler failed", "error", err)
		}
	}
}

// Debug logs at debug level
func (l *StructuredLogger) Debug(ctx context.Context, message string, attrs ...interface{}) {
	l.LogWithLevel(ctx, DebugLevel, message, attrs...)
}

// Info logs at info level
func (l *StructuredLogger) Info(ctx context.Context, message string, attrs ...interface{}) {
	l.LogWithLevel(ctx, InfoLevel, message, attrs...)
}

// Warn logs at warn level
func (l *StructuredLogger) Warn(ctx context.Context, message string, attrs ...interface{}) {
	l.LogWithLevel(ctx, WarnLevel, message, attrs...)
}

// Error logs at error level
func (l *StructuredLogger) Error(ctx context.Context, message string, attrs ...interface{}) {
	l.LogWithLevel(ctx, ErrorLevel, message, attrs...)
}

// DebugWithContext logs at debug level with enhanced context
func (l *StructuredLogger) DebugWithContext(message string, args ...interface{}) {
	l.LogWithLevel(context.Background(), DebugLevel, message, args...)
}

// InfoWithContext logs at info level with enhanced context
func (l *StructuredLogger) InfoWithContext(message string, args ...interface{}) {
	l.LogWithLevel(context.Background(), InfoLevel, message, args...)
}

// WarnWithContext logs at warn level with enhanced context
func (l *StructuredLogger) WarnWithContext(message string, args ...interface{}) {
	l.LogWithLevel(context.Background(), WarnLevel, message, args...)
}

// ErrorWithContext logs at error level with enhanced context
func (l *StructuredLogger) ErrorWithContext(message string, args ...interface{}) {
	l.LogWithLevel(context.Background(), ErrorLevel, message, args...)
}

// shouldLog checks if message should be logged based on level
func (l *StructuredLogger) shouldLog(level LogLevel) bool {
	return level >= l.config.Level
}

// passesFilters checks if record passes all filters
func (l *StructuredLogger) passesFilters(record *LogRecord) bool {
	attrs := make([]slog.Attr, 0, len(record.Attrs))
	for k, v := range record.Attrs {
		attrs = append(attrs, slog.Any(k, v))
	}

	for _, filter := range l.filters {
		if !filter.ShouldLog(record.Level, record.Message, attrs) {
			return false
		}
	}
	return true
}

// buildAttrs builds attributes map from variadic arguments
func (l *StructuredLogger) buildAttrs(attrs ...interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Add metadata fields
	for k, v := range l.metadata {
		result[k] = v
	}

	// Add provided attributes
	for i := 0; i < len(attrs); i += 2 {
		if i+1 < len(attrs) {
			if key, ok := attrs[i].(string); ok {
				result[key] = attrs[i+1]
			}
		}
	}

	return result
}

// convertToSlogAttrs converts internal attrs to slog attrs
func (l *StructuredLogger) convertToSlogAttrs(record *LogRecord) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(record.Attrs))
	
	for k, v := range record.Attrs {
		// Truncate long values if configured
		if l.config.MaxFieldLength > 0 {
			if str, ok := v.(string); ok && len(str) > l.config.MaxFieldLength {
				v = str[:l.config.MaxFieldLength] + "..."
			}
		}
		attrs = append(attrs, slog.Any(k, v))
	}

	return attrs
}

// getCaller gets caller information
func (l *StructuredLogger) getCaller(skip int) *CallerInfo {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return nil
	}

	function := runtime.FuncForPC(pc).Name()
	
	// Clean up file path - show only filename
	if lastSlash := strings.LastIndex(file, "/"); lastSlash >= 0 {
		file = file[lastSlash+1:]
	}

	return &CallerInfo{
		Function: function,
		File:     file,
		Line:     line,
	}
}

// AddFilter adds a log filter
func (l *StructuredLogger) AddFilter(filter LogFilter) {
	l.filters = append(l.filters, filter)
}

// AddEnricher adds a log enricher
func (l *StructuredLogger) AddEnricher(enricher LogEnricher) {
	l.enrichers = append(l.enrichers, enricher)
}

// AddHandler adds a log handler
func (l *StructuredLogger) AddHandler(handler LogHandler) {
	l.handlers = append(l.handlers, handler)
}

// SetLevel sets the log level
func (l *StructuredLogger) SetLevel(level LogLevel) {
	l.config.Level = level
}

// GetLevel returns the current log level
func (l *StructuredLogger) GetLevel() LogLevel {
	return l.config.Level
}

// IsLevelEnabled checks if a log level is enabled
func (l *StructuredLogger) IsLevelEnabled(level LogLevel) bool {
	return level >= l.config.Level
}

// Flush flushes any buffered logs (useful for handlers that buffer)
func (l *StructuredLogger) Flush() error {
	// In this implementation, slog handles flushing automatically
	// But we can extend this for custom handlers that need explicit flushing
	return nil
}

// Close closes the logger and any associated resources
func (l *StructuredLogger) Close() error {
	// Close output file if it's not stdout/stderr
	if l.config.Output != os.Stdout && l.config.Output != os.Stderr {
		return l.config.Output.Close()
	}
	return nil
}

// GetConfig returns the logger configuration
func (l *StructuredLogger) GetConfig() *LoggerConfig {
	return l.config
}

// Clone creates a copy of the logger
func (l *StructuredLogger) Clone() *StructuredLogger {
	newLogger := &StructuredLogger{
		Logger:   l.Logger,
		config:   l.config,
		filters:  make([]LogFilter, len(l.filters)),
		enrichers: make([]LogEnricher, len(l.enrichers)),
		handlers: make([]LogHandler, len(l.handlers)),
		metadata: make(map[string]interface{}),
	}

	// Copy slices
	copy(newLogger.filters, l.filters)
	copy(newLogger.enrichers, l.enrichers)
	copy(newLogger.handlers, l.handlers)

	// Copy metadata
	for k, v := range l.metadata {
		newLogger.metadata[k] = v
	}

	return newLogger
}

// LogMetrics logs performance metrics
func (l *StructuredLogger) LogMetrics(ctx context.Context, operation string, duration time.Duration, metadata map[string]interface{}) {
	attrs := map[string]interface{}{
		"operation":      operation,
		"duration_ms":    duration.Milliseconds(),
		"duration_ns":    duration.Nanoseconds(),
		"metric_type":    "performance",
	}

	for k, v := range metadata {
		attrs[k] = v
	}

	record := &LogRecord{
		Level:     InfoLevel,
		Message:   fmt.Sprintf("Operation completed: %s", operation),
		Timestamp: time.Now(),
		Attrs:     attrs,
		Context:   ctx,
	}

	// Use special metrics handler if available
	for _, handler := range l.handlers {
		if metricsHandler, ok := handler.(MetricsHandler); ok {
			if err := metricsHandler.HandleMetrics(record); err != nil {
				l.Logger.Error("Metrics handler failed", "error", err)
			}
			return
		}
	}

	// Fall back to regular logging
	l.LogWithLevel(ctx, InfoLevel, record.Message, l.attrsToVariadic(record.Attrs)...)
}

// attrsToVariadic converts attrs map to variadic arguments
func (l *StructuredLogger) attrsToVariadic(attrs map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(attrs)*2)
	for k, v := range attrs {
		result = append(result, k, v)
	}
	return result
}

// MetricsHandler is a specialized handler for metrics
type MetricsHandler interface {
	LogHandler
	HandleMetrics(record *LogRecord) error
}