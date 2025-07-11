package utils

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	logger "pre-processor/utils/logger"
)

// AppMetrics tracks application-level metrics
type AppMetrics struct {
	// Counters
	ArticlesProcessed  atomic.Int64
	FeedsProcessed     atomic.Int64
	SummariesGenerated atomic.Int64
	Errors             map[string]*atomic.Int64

	// Timing statistics
	ProcessingDuration *DurationStats
	HTTPDuration       *DurationStats
	DBQueryDuration    *DurationStats

	// Gauges
	ActiveGoroutines atomic.Int32
	MemoryUsage      atomic.Int64
	QueueDepth       map[string]*atomic.Int32

	// Size statistics
	ArticleSize  *SizeStats
	ResponseSize *SizeStats

	mu sync.RWMutex
}

// NewAppMetrics creates a new application metrics instance
func NewAppMetrics() *AppMetrics {
	defaultBuckets := []time.Duration{
		1 * time.Millisecond,
		5 * time.Millisecond,
		10 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
	}

	return &AppMetrics{
		Errors:             make(map[string]*atomic.Int64),
		ProcessingDuration: NewDurationStats(defaultBuckets),
		HTTPDuration:       NewDurationStats(defaultBuckets),
		DBQueryDuration:    NewDurationStats(defaultBuckets),
		QueueDepth:         make(map[string]*atomic.Int32),
		ArticleSize:        NewSizeStats(),
		ResponseSize:       NewSizeStats(),
	}
}

// Counter methods
func (m *AppMetrics) IncrementArticlesProcessed() {
	m.ArticlesProcessed.Add(1)
}

func (m *AppMetrics) IncrementFeedsProcessed() {
	m.FeedsProcessed.Add(1)
}

func (m *AppMetrics) IncrementSummariesGenerated() {
	m.SummariesGenerated.Add(1)
}

func (m *AppMetrics) IncrementError(errorType string) {
	m.mu.Lock()
	if counter, exists := m.Errors[errorType]; exists {
		m.mu.Unlock()
		counter.Add(1)
		return
	}

	counter := &atomic.Int64{}
	m.Errors[errorType] = counter
	m.mu.Unlock()
	counter.Add(1)
}

func (m *AppMetrics) GetErrorCount(errorType string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if counter, exists := m.Errors[errorType]; exists {
		return counter.Load()
	}
	return 0
}

// Gauge methods
func (m *AppMetrics) SetActiveGoroutines(count int32) {
	m.ActiveGoroutines.Store(count)
}

func (m *AppMetrics) SetMemoryUsage(bytes int64) {
	m.MemoryUsage.Store(bytes)
}

func (m *AppMetrics) SetQueueDepth(queueName string, depth int32) {
	m.mu.Lock()
	if counter, exists := m.QueueDepth[queueName]; exists {
		m.mu.Unlock()
		counter.Store(depth)
		return
	}

	counter := &atomic.Int32{}
	m.QueueDepth[queueName] = counter
	m.mu.Unlock()
	counter.Store(depth)
}

func (m *AppMetrics) GetQueueDepth(queueName string) int32 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if counter, exists := m.QueueDepth[queueName]; exists {
		return counter.Load()
	}
	return 0
}

// DurationStats tracks timing statistics with histogram buckets
type DurationStats struct {
	count   atomic.Int64
	total   atomic.Int64 // microseconds
	min     atomic.Int64
	max     atomic.Int64
	buckets []int64 // microsecond boundaries
	counts  []atomic.Int64
}

// NewDurationStats creates duration statistics with specified buckets
func NewDurationStats(buckets []time.Duration) *DurationStats {
	bucketsMicros := make([]int64, len(buckets))
	counts := make([]atomic.Int64, len(buckets)+1)

	for i, d := range buckets {
		bucketsMicros[i] = d.Microseconds()
	}

	stats := &DurationStats{
		buckets: bucketsMicros,
		counts:  counts,
	}
	stats.min.Store(math.MaxInt64)
	return stats
}

// Record adds a duration measurement
func (d *DurationStats) Record(duration time.Duration) {
	micros := duration.Microseconds()

	d.count.Add(1)
	d.total.Add(micros)

	// Update min/max atomically
	for {
		oldMin := d.min.Load()
		if micros >= oldMin || d.min.CompareAndSwap(oldMin, micros) {
			break
		}
	}

	for {
		oldMax := d.max.Load()
		if micros <= oldMax || d.max.CompareAndSwap(oldMax, micros) {
			break
		}
	}

	// Update histogram buckets
	for i, boundary := range d.buckets {
		if micros <= boundary {
			d.counts[i].Add(1)
			return
		}
	}
	d.counts[len(d.counts)-1].Add(1)
}

// StatsSnapshot represents a point-in-time view of duration statistics
type StatsSnapshot struct {
	Count int64   `json:"count"`
	AvgMs float64 `json:"avg_ms"`
	MinMs float64 `json:"min_ms"`
	MaxMs float64 `json:"max_ms"`
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
}

// GetStats returns a snapshot of current statistics
func (d *DurationStats) GetStats() StatsSnapshot {
	count := d.count.Load()
	if count == 0 {
		return StatsSnapshot{}
	}

	total := d.total.Load()
	min := d.min.Load()
	max := d.max.Load()

	avgMs := float64(total) / float64(count) / 1000.0
	minMs := float64(min) / 1000.0
	maxMs := float64(max) / 1000.0

	// Simple percentile approximation using buckets
	p50Ms := d.approximatePercentile(0.50)
	p95Ms := d.approximatePercentile(0.95)
	p99Ms := d.approximatePercentile(0.99)

	return StatsSnapshot{
		Count: count,
		AvgMs: avgMs,
		MinMs: minMs,
		MaxMs: maxMs,
		P50Ms: p50Ms,
		P95Ms: p95Ms,
		P99Ms: p99Ms,
	}
}

// approximatePercentile provides a rough percentile estimation using histogram buckets
func (d *DurationStats) approximatePercentile(percentile float64) float64 {
	count := d.count.Load()
	if count == 0 {
		return 0
	}

	target := int64(float64(count) * percentile)
	cumulative := int64(0)

	for i := range d.counts {
		cumulative += d.counts[i].Load()
		if cumulative >= target {
			if i < len(d.buckets) {
				return float64(d.buckets[i]) / 1000.0 // Convert to milliseconds
			}
			// Return last bucket boundary if in overflow bucket
			if len(d.buckets) > 0 {
				return float64(d.buckets[len(d.buckets)-1]) / 1000.0
			}
			return 0
		}
	}

	return 0
}

// SizeStats tracks size statistics (for articles, responses, etc.)
type SizeStats struct {
	count atomic.Int64
	total atomic.Int64
	min   atomic.Int64
	max   atomic.Int64
}

// NewSizeStats creates a new size statistics tracker
func NewSizeStats() *SizeStats {
	stats := &SizeStats{}
	stats.min.Store(math.MaxInt64)
	return stats
}

// Record adds a size measurement
func (s *SizeStats) Record(size int64) {
	s.count.Add(1)
	s.total.Add(size)

	// Update min/max atomically
	for {
		oldMin := s.min.Load()
		if size >= oldMin || s.min.CompareAndSwap(oldMin, size) {
			break
		}
	}

	for {
		oldMax := s.max.Load()
		if size <= oldMax || s.max.CompareAndSwap(oldMax, size) {
			break
		}
	}
}

// RuntimeMetrics collects Go runtime metrics
type RuntimeMetrics struct {
	lastGC       time.Time
	lastMemStats runtime.MemStats
	mu           sync.RWMutex
}

// NewRuntimeMetrics creates a new runtime metrics collector
func NewRuntimeMetrics() *RuntimeMetrics {
	return &RuntimeMetrics{}
}

// RuntimeSnapshot represents a point-in-time view of runtime metrics
type RuntimeSnapshot struct {
	Timestamp  time.Time `json:"timestamp"`
	Goroutines int       `json:"goroutines"`
	MemAllocMB float64   `json:"mem_alloc_mb"`
	MemSysMB   float64   `json:"mem_sys_mb"`
	MemHeapMB  float64   `json:"mem_heap_mb"`
	GCPauseMs  float64   `json:"gc_pause_ms"`
	GCCount    uint32    `json:"gc_count"`
	GCLastTime time.Time `json:"gc_last_time"`
}

// Collect captures current runtime metrics
func (r *RuntimeMetrics) Collect() RuntimeSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	r.lastMemStats = m

	// Safe conversion of uint64 to int64 for GC last time
	var gcLastTime time.Time
	if m.LastGC > 0 {
		// Check for potential overflow when converting uint64 to int64
		if m.LastGC <= math.MaxInt64 {
			gcLastTime = time.Unix(0, int64(m.LastGC))
		} else {
			// If overflow would occur, use current time as fallback
			gcLastTime = time.Now()
		}
	}

	return RuntimeSnapshot{
		Timestamp:  time.Now(),
		Goroutines: runtime.NumGoroutine(),
		MemAllocMB: float64(m.Alloc) / (1024 * 1024),
		MemSysMB:   float64(m.Sys) / (1024 * 1024),
		MemHeapMB:  float64(m.HeapAlloc) / (1024 * 1024),
		GCPauseMs:  float64(m.PauseNs[(m.NumGC+255)%256]) / 1e6,
		GCCount:    m.NumGC,
		GCLastTime: gcLastTime,
	}
}

// StartCollector starts periodic runtime metrics collection
func (r *RuntimeMetrics) StartCollector(ctx context.Context, interval time.Duration, customLogger *slog.Logger) {
	if customLogger == nil {
		customLogger = logger.Logger // Use default logger if none provided
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			snapshot := r.Collect()

			// Output metrics in rask-log-forwarder compatible JSON format
			customLogger.Info("runtime_metrics",
				"timestamp", snapshot.Timestamp.UTC().Format(time.RFC3339Nano),
				"service", getEnvOrDefault("SERVICE_NAME", "pre-processor"),
				"service_version", getEnvOrDefault("SERVICE_VERSION", "1.0.0"),
				"metric_type", "runtime",
				"metrics", map[string]interface{}{
					"runtime": map[string]interface{}{
						"goroutines": snapshot.Goroutines,
						"memory": map[string]interface{}{
							"alloc_mb": snapshot.MemAllocMB,
							"sys_mb":   snapshot.MemSysMB,
							"heap_mb":  snapshot.MemHeapMB,
						},
						"gc": map[string]interface{}{
							"pause_ms":  snapshot.GCPauseMs,
							"count":     snapshot.GCCount,
							"last_time": snapshot.GCLastTime.Format(time.RFC3339Nano),
						},
					},
				},
			)

			// Warning for high GC frequency
			if snapshot.GCCount > 0 && time.Since(snapshot.GCLastTime) < 10*time.Second {
				customLogger.Warn("high GC frequency detected",
					"gc_count", snapshot.GCCount,
					"time_since_last_gc", time.Since(snapshot.GCLastTime),
				)
			}

		case <-ctx.Done():
			return
		}
	}
}

// SimpleTracer provides basic tracing functionality
type SimpleTracer struct {
	logger *slog.Logger
}

// NewSimpleTracer creates a simple tracer
func NewSimpleTracer(customLogger *slog.Logger) *SimpleTracer {
	if customLogger == nil {
		customLogger = logger.Logger
	}
	return &SimpleTracer{
		logger: customLogger,
	}
}

// Span represents a trace span
type Span struct {
	name       string
	startTime  time.Time
	attributes map[string]interface{}
	logger     *slog.Logger
}

// StartSpan begins a new trace span
func (t *SimpleTracer) StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	traceID := getOrGenerateTraceID(ctx)
	spanID := generateSpanID()

	span := &Span{
		name:       name,
		startTime:  time.Now(),
		attributes: make(map[string]interface{}),
		logger: t.logger.With(
			"trace_id", traceID,
			"span_id", spanID,
			"span_name", name,
		),
	}

	if t.logger != nil {
		span.logger.Info("span started")
	}

	ctx = context.WithValue(ctx, "current_span", span)
	return ctx, span
}

// SetAttributes adds attributes to the span
func (s *Span) SetAttributes(attrs ...interface{}) {
	if len(attrs)%2 != 0 {
		return
	}

	for i := 0; i < len(attrs); i += 2 {
		key, ok := attrs[i].(string)
		if ok {
			s.attributes[key] = attrs[i+1]
		}
	}
}

// End completes the span
func (s *Span) End() {
	duration := time.Since(s.startTime)

	args := []interface{}{
		"duration_ms", duration.Milliseconds(),
	}

	for k, v := range s.attributes {
		args = append(args, k, v)
	}

	if s.logger != nil {
		s.logger.Info("span ended", args...)
	}
}

// PerformanceReporter generates periodic performance reports
type PerformanceReporter struct {
	metrics  *AppMetrics
	runtime  *RuntimeMetrics
	logger   *slog.Logger
	interval time.Duration
}

// NewPerformanceReporter creates a performance reporter
func NewPerformanceReporter(metrics *AppMetrics, runtime *RuntimeMetrics, customLogger *slog.Logger, interval time.Duration) *PerformanceReporter {
	if customLogger == nil {
		customLogger = logger.Logger
	}
	return &PerformanceReporter{
		metrics:  metrics,
		runtime:  runtime,
		logger:   customLogger,
		interval: interval,
	}
}

// PerformanceReport represents a performance report
type PerformanceReport struct {
	Timestamp time.Time       `json:"timestamp"`
	Metrics   MetricsSummary  `json:"metrics"`
	Runtime   RuntimeSnapshot `json:"runtime"`
}

// MetricsSummary summarizes application metrics
type MetricsSummary struct {
	ArticlesProcessed  int64         `json:"articles_processed"`
	FeedsProcessed     int64         `json:"feeds_processed"`
	SummariesGenerated int64         `json:"summaries_generated"`
	ProcessingStats    StatsSnapshot `json:"processing_stats"`
	DBQueryStats       StatsSnapshot `json:"db_query_stats"`
}

// GenerateReport creates a performance report
func (r *PerformanceReporter) GenerateReport() PerformanceReport {
	runtimeSnapshot := r.runtime.Collect()

	return PerformanceReport{
		Timestamp: time.Now(),
		Metrics: MetricsSummary{
			ArticlesProcessed:  r.metrics.ArticlesProcessed.Load(),
			FeedsProcessed:     r.metrics.FeedsProcessed.Load(),
			SummariesGenerated: r.metrics.SummariesGenerated.Load(),
			ProcessingStats:    r.metrics.ProcessingDuration.GetStats(),
			DBQueryStats:       r.metrics.DBQueryDuration.GetStats(),
		},
		Runtime: runtimeSnapshot,
	}
}

// StartReporting begins periodic performance reporting
func (r *PerformanceReporter) StartReporting(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			report := r.GenerateReport()
			r.logReport(report)

		case <-ctx.Done():
			return
		}
	}
}

// logReport outputs the performance report
func (r *PerformanceReporter) logReport(report PerformanceReport) {
	if r.logger == nil {
		return
	}

	r.logger.Info("performance_report",
		"timestamp", report.Timestamp.UTC().Format(time.RFC3339Nano),
		"service", getEnvOrDefault("SERVICE_NAME", "pre-processor"),
		"metric_type", "performance",
		"articles_processed", report.Metrics.ArticlesProcessed,
		"feeds_processed", report.Metrics.FeedsProcessed,
		"summaries_generated", report.Metrics.SummariesGenerated,
		"avg_processing_ms", report.Metrics.ProcessingStats.AvgMs,
		"p95_processing_ms", report.Metrics.ProcessingStats.P95Ms,
		"goroutines", report.Runtime.Goroutines,
		"memory_mb", report.Runtime.MemAllocMB,
	)
}

// Utility functions

func getOrGenerateTraceID(ctx context.Context) string {
	if traceID := ctx.Value("trace_id"); traceID != nil {
		if id, ok := traceID.(string); ok {
			return id
		}
	}
	return generateTraceID()
}

func generateTraceID() string {
	bytes := make([]byte, 16)
	_, err := rand.Read(bytes)
	if err != nil {
		// Fallback to time-based ID if crypto/rand fails
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", bytes)
}

func generateSpanID() string {
	bytes := make([]byte, 8)
	_, err := rand.Read(bytes)
	if err != nil {
		// Fallback to time-based ID if crypto/rand fails
		return fmt.Sprintf("%x", time.Now().UnixNano()&0xFFFFFFFF)
	}
	return fmt.Sprintf("%x", bytes)
}
