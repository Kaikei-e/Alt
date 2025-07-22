// Phase R4: ログコンテキスト管理 - リクエストトレーシング・コンテキスト伝播
package logging

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ContextKey represents keys for context values
type ContextKey string

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey ContextKey = "request_id"
	// TraceIDKey is the context key for trace ID
	TraceIDKey ContextKey = "trace_id"
	// SpanIDKey is the context key for span ID
	SpanIDKey ContextKey = "span_id"
	// UserIDKey is the context key for user ID
	UserIDKey ContextKey = "user_id"
	// OperationKey is the context key for operation name
	OperationKey ContextKey = "operation"
	// CorrelationIDKey is the context key for correlation ID
	CorrelationIDKey ContextKey = "correlation_id"
)

// LogContextManager manages logging contexts and correlation across operations
type LogContextManager struct {
	spans    map[string]*Span
	mutex    sync.RWMutex
	logger   *StructuredLogger
	enricher *ContextEnricher
}

// Span represents a trace span for context propagation
type Span struct {
	SpanID       string
	TraceID      string
	ParentSpanID string
	Operation    string
	StartTime    time.Time
	EndTime      *time.Time
	Status       SpanStatus
	Tags         map[string]interface{}
	Logs         []*SpanLog
	mutex        sync.RWMutex
}

// SpanStatus represents the status of a span
type SpanStatus string

const (
	// SpanStatusActive indicates an active span
	SpanStatusActive SpanStatus = "active"
	// SpanStatusSuccess indicates a successful span
	SpanStatusSuccess SpanStatus = "success"
	// SpanStatusError indicates a failed span
	SpanStatusError SpanStatus = "error"
	// SpanStatusCancelled indicates a cancelled span
	SpanStatusCancelled SpanStatus = "cancelled"
)

// SpanLog represents a log entry within a span
type SpanLog struct {
	Timestamp time.Time
	Level     LogLevel
	Message   string
	Fields    map[string]interface{}
}

// NewLogContextManager creates a new log context manager
func NewLogContextManager(logger *StructuredLogger) *LogContextManager {
	manager := &LogContextManager{
		spans:  make(map[string]*Span),
		logger: logger,
	}

	// Create and add context enricher
	manager.enricher = &ContextEnricher{manager: manager}
	if logger != nil {
		logger.AddEnricher(manager.enricher)
	}

	return manager
}

// StartOperation starts a new operation with tracing context
func (m *LogContextManager) StartOperation(ctx context.Context, operation string) (context.Context, *Span) {
	requestID := m.getOrGenerateRequestID(ctx)
	traceID := m.getOrGenerateTraceID(ctx)
	spanID := m.generateSpanID()

	// Get parent span ID if exists
	var parentSpanID string
	if existingSpanID := m.getSpanID(ctx); existingSpanID != "" {
		parentSpanID = existingSpanID
	}

	span := &Span{
		SpanID:       spanID,
		TraceID:      traceID,
		ParentSpanID: parentSpanID,
		Operation:    operation,
		StartTime:    time.Now(),
		Status:       SpanStatusActive,
		Tags:         make(map[string]interface{}),
		Logs:         make([]*SpanLog, 0),
	}

	// Store span
	m.mutex.Lock()
	m.spans[spanID] = span
	m.mutex.Unlock()

	// Create new context with span information
	newCtx := context.WithValue(ctx, RequestIDKey, requestID)
	newCtx = context.WithValue(newCtx, TraceIDKey, traceID)
	newCtx = context.WithValue(newCtx, SpanIDKey, spanID)
	newCtx = context.WithValue(newCtx, OperationKey, operation)

	// Log operation start
	if m.logger != nil {
		m.logger.Info(newCtx, fmt.Sprintf("Operation started: %s", operation),
			"span_id", spanID,
			"trace_id", traceID,
			"parent_span_id", parentSpanID,
		)
	}

	return newCtx, span
}

// FinishOperation finishes an operation and logs the result
func (m *LogContextManager) FinishOperation(ctx context.Context, span *Span, err error) {
	if span == nil {
		return
	}

	span.mutex.Lock()
	endTime := time.Now()
	span.EndTime = &endTime
	
	if err != nil {
		span.Status = SpanStatusError
		span.Tags["error"] = err.Error()
	} else {
		span.Status = SpanStatusSuccess
	}
	span.mutex.Unlock()

	duration := endTime.Sub(span.StartTime)

	// Log operation completion
	if m.logger != nil {
		level := InfoLevel
		if err != nil {
			level = ErrorLevel
		}

		attrs := []interface{}{
			"span_id", span.SpanID,
			"trace_id", span.TraceID,
			"duration_ms", duration.Milliseconds(),
			"status", span.Status,
		}

		if err != nil {
			attrs = append(attrs, "error", err.Error())
		}

		message := fmt.Sprintf("Operation completed: %s", span.Operation)
		m.logger.LogWithLevel(ctx, level, message, attrs...)
	}

	// Clean up span after a delay
	go m.cleanupSpan(span.SpanID, 5*time.Minute)
}

// StartChildOperation starts a child operation within an existing context
func (m *LogContextManager) StartChildOperation(ctx context.Context, operation string) (context.Context, *Span) {
	return m.StartOperation(ctx, operation)
}

// AddSpanTag adds a tag to the current span
func (m *LogContextManager) AddSpanTag(ctx context.Context, key string, value interface{}) {
	spanID := m.getSpanID(ctx)
	if spanID == "" {
		return
	}

	m.mutex.RLock()
	span, exists := m.spans[spanID]
	m.mutex.RUnlock()

	if !exists {
		return
	}

	span.mutex.Lock()
	span.Tags[key] = value
	span.mutex.Unlock()
}

// LogToSpan logs a message to the current span
func (m *LogContextManager) LogToSpan(ctx context.Context, level LogLevel, message string, fields map[string]interface{}) {
	spanID := m.getSpanID(ctx)
	if spanID == "" {
		return
	}

	m.mutex.RLock()
	span, exists := m.spans[spanID]
	m.mutex.RUnlock()

	if !exists {
		return
	}

	spanLog := &SpanLog{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Fields:    fields,
	}

	span.mutex.Lock()
	span.Logs = append(span.Logs, spanLog)
	span.mutex.Unlock()
}

// GetSpan retrieves the current span from context
func (m *LogContextManager) GetSpan(ctx context.Context) *Span {
	spanID := m.getSpanID(ctx)
	if spanID == "" {
		return nil
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.spans[spanID]
}

// GetTraceSpans returns all spans for a trace
func (m *LogContextManager) GetTraceSpans(traceID string) []*Span {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var spans []*Span
	for _, span := range m.spans {
		if span.TraceID == traceID {
			spans = append(spans, span)
		}
	}
	return spans
}

// GetActiveSpans returns all currently active spans
func (m *LogContextManager) GetActiveSpans() []*Span {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var spans []*Span
	for _, span := range m.spans {
		if span.Status == SpanStatusActive {
			spans = append(spans, span)
		}
	}
	return spans
}

// WithCorrelationID adds correlation ID to context
func (m *LogContextManager) WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// WithUserID adds user ID to context
func (m *LogContextManager) WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// ExtractContextFields extracts all context fields as a map
func (m *LogContextManager) ExtractContextFields(ctx context.Context) map[string]interface{} {
	fields := make(map[string]interface{})

	if requestID := m.getRequestID(ctx); requestID != "" {
		fields["request_id"] = requestID
	}
	if traceID := m.getTraceID(ctx); traceID != "" {
		fields["trace_id"] = traceID
	}
	if spanID := m.getSpanID(ctx); spanID != "" {
		fields["span_id"] = spanID
	}
	if userID := m.getUserID(ctx); userID != "" {
		fields["user_id"] = userID
	}
	if operation := m.getOperation(ctx); operation != "" {
		fields["operation"] = operation
	}
	if correlationID := m.getCorrelationID(ctx); correlationID != "" {
		fields["correlation_id"] = correlationID
	}

	return fields
}

// Helper methods for context value extraction

func (m *LogContextManager) getRequestID(ctx context.Context) string {
	if val := ctx.Value(RequestIDKey); val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (m *LogContextManager) getTraceID(ctx context.Context) string {
	if val := ctx.Value(TraceIDKey); val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (m *LogContextManager) getSpanID(ctx context.Context) string {
	if val := ctx.Value(SpanIDKey); val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (m *LogContextManager) getUserID(ctx context.Context) string {
	if val := ctx.Value(UserIDKey); val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (m *LogContextManager) getOperation(ctx context.Context) string {
	if val := ctx.Value(OperationKey); val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (m *LogContextManager) getCorrelationID(ctx context.Context) string {
	if val := ctx.Value(CorrelationIDKey); val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func (m *LogContextManager) getOrGenerateRequestID(ctx context.Context) string {
	if requestID := m.getRequestID(ctx); requestID != "" {
		return requestID
	}
	return m.generateRequestID()
}

func (m *LogContextManager) getOrGenerateTraceID(ctx context.Context) string {
	if traceID := m.getTraceID(ctx); traceID != "" {
		return traceID
	}
	return m.generateTraceID()
}

// ID generation methods

func (m *LogContextManager) generateRequestID() string {
	return fmt.Sprintf("req_%s", uuid.New().String()[:8])
}

func (m *LogContextManager) generateTraceID() string {
	return fmt.Sprintf("trace_%s", uuid.New().String()[:12])
}

func (m *LogContextManager) generateSpanID() string {
	return fmt.Sprintf("span_%s", uuid.New().String()[:8])
}

// cleanupSpan removes span from memory after delay
func (m *LogContextManager) cleanupSpan(spanID string, delay time.Duration) {
	time.Sleep(delay)
	
	m.mutex.Lock()
	delete(m.spans, spanID)
	m.mutex.Unlock()
}

// ContextEnricher enriches log records with context information
type ContextEnricher struct {
	manager *LogContextManager
}

// Enrich implements LogEnricher interface
func (e *ContextEnricher) Enrich(ctx context.Context, record *LogRecord) error {
	// Extract context fields and add to record attributes
	contextFields := e.manager.ExtractContextFields(ctx)
	
	for k, v := range contextFields {
		record.Attrs[k] = v
	}

	// Add span tags if span exists
	if span := e.manager.GetSpan(ctx); span != nil {
		span.mutex.RLock()
		for k, v := range span.Tags {
			record.Attrs[fmt.Sprintf("span_tag_%s", k)] = v
		}
		span.mutex.RUnlock()

		// Log to span
		fields := make(map[string]interface{})
		for k, v := range record.Attrs {
			fields[k] = v
		}
		
		e.manager.LogToSpan(ctx, record.Level, record.Message, fields)
	}

	return nil
}

// GetMetrics returns context manager metrics
func (m *LogContextManager) GetMetrics() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	totalSpans := len(m.spans)
	activeSpans := 0
	completedSpans := 0
	errorSpans := 0

	for _, span := range m.spans {
		switch span.Status {
		case SpanStatusActive:
			activeSpans++
		case SpanStatusSuccess:
			completedSpans++
		case SpanStatusError:
			errorSpans++
		}
	}

	return map[string]interface{}{
		"total_spans":     totalSpans,
		"active_spans":    activeSpans,
		"completed_spans": completedSpans,
		"error_spans":     errorSpans,
	}
}