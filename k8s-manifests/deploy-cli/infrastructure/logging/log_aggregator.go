// Phase R4: ログ集約 - ログ集約・ローテーション・外部システム連携
package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// LogAggregator aggregates and manages log data from multiple sources
type LogAggregator struct {
	config          *AggregatorConfig
	outputs         []OutputHandler
	buffer          *LogBuffer
	rotator         *LogRotator
	exporter        *LogExporter
	metrics         *AggregatorMetrics
	stopCh          chan struct{}
	flushTicker     *time.Ticker
	mutex           sync.RWMutex
	isRunning       bool
}

// AggregatorConfig holds aggregator configuration
type AggregatorConfig struct {
	BufferSize       int
	FlushInterval    time.Duration
	MaxRetries       int
	RetryDelay       time.Duration
	CompressionType  CompressionType
	EnableRotation   bool
	RotationConfig   *RotationConfig
	ExportConfig     *ExportConfig
}

// CompressionType represents compression algorithms
type CompressionType string

const (
	NoCompression   CompressionType = "none"
	GzipCompression CompressionType = "gzip"
	LZ4Compression  CompressionType = "lz4"
)

// RotationConfig holds log rotation configuration
type RotationConfig struct {
	MaxSize        int64         // Maximum size in bytes
	MaxAge         time.Duration // Maximum age
	MaxBackups     int          // Maximum number of backup files
	LocalTime      bool         // Use local time for rotation
	Compress       bool         // Compress rotated files
}

// ExportConfig holds log export configuration
type ExportConfig struct {
	Enabled       bool
	ExportFormat  string
	Destination   string
	BatchSize     int
	ExportInterval time.Duration
}

// OutputHandler handles log output to different destinations
type OutputHandler interface {
	Write(records []*LogRecord) error
	Close() error
}

// LogBuffer buffers log records for batch processing
type LogBuffer struct {
	records []LogRecord
	mutex   sync.RWMutex
	maxSize int
}

// LogRotator handles log file rotation
type LogRotator struct {
	config    *RotationConfig
	currentFile *os.File
	currentSize int64
	mutex       sync.RWMutex
}

// LogExporter exports logs to external systems
type LogExporter struct {
	config  *ExportConfig
	client  ExportClient
	buffer  []LogRecord
	mutex   sync.RWMutex
}

// ExportClient interface for external log systems
type ExportClient interface {
	Export(records []LogRecord) error
	Close() error
}

// AggregatorMetrics tracks aggregator performance
type AggregatorMetrics struct {
	TotalRecords     int64
	BufferedRecords  int64
	ExportedRecords  int64
	FailedExports    int64
	RotationCount    int64
	LastFlushTime    time.Time
	LastRotationTime time.Time
	mutex            sync.RWMutex
}

// NewLogAggregator creates a new log aggregator
func NewLogAggregator(config *AggregatorConfig) *LogAggregator {
	if config == nil {
		config = DefaultAggregatorConfig()
	}

	aggregator := &LogAggregator{
		config:  config,
		buffer:  NewLogBuffer(config.BufferSize),
		metrics: &AggregatorMetrics{},
		stopCh:  make(chan struct{}),
	}

	// Initialize rotator if enabled
	if config.EnableRotation && config.RotationConfig != nil {
		aggregator.rotator = NewLogRotator(config.RotationConfig)
	}

	// Initialize exporter if enabled
	if config.ExportConfig != nil && config.ExportConfig.Enabled {
		aggregator.exporter = NewLogExporter(config.ExportConfig)
	}

	return aggregator
}

// DefaultAggregatorConfig returns default aggregator configuration
func DefaultAggregatorConfig() *AggregatorConfig {
	return &AggregatorConfig{
		BufferSize:      1000,
		FlushInterval:   30 * time.Second,
		MaxRetries:      3,
		RetryDelay:      1 * time.Second,
		CompressionType: NoCompression,
		EnableRotation:  false,
	}
}

// Start starts the log aggregator
func (a *LogAggregator) Start() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.isRunning {
		return fmt.Errorf("aggregator is already running")
	}

	// Start flush ticker
	a.flushTicker = time.NewTicker(a.config.FlushInterval)

	// Start background goroutine
	go a.run()

	a.isRunning = true
	return nil
}

// Stop stops the log aggregator
func (a *LogAggregator) Stop() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if !a.isRunning {
		return nil
	}

	// Stop ticker
	if a.flushTicker != nil {
		a.flushTicker.Stop()
	}

	// Signal stop
	close(a.stopCh)

	// Final flush
	a.flush()

	// Close outputs
	for _, output := range a.outputs {
		output.Close()
	}

	// Close exporter
	if a.exporter != nil {
		a.exporter.Close()
	}

	// Close rotator
	if a.rotator != nil {
		a.rotator.Close()
	}

	a.isRunning = false
	return nil
}

// AddRecord adds a log record to the aggregator
func (a *LogAggregator) AddRecord(record *LogRecord) {
	a.buffer.Add(*record)
	
	a.metrics.mutex.Lock()
	a.metrics.TotalRecords++
	a.metrics.BufferedRecords++
	a.metrics.mutex.Unlock()
}

// AddOutput adds an output handler
func (a *LogAggregator) AddOutput(output OutputHandler) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	
	a.outputs = append(a.outputs, output)
}

// run is the main aggregator loop
func (a *LogAggregator) run() {
	for {
		select {
		case <-a.flushTicker.C:
			a.flush()
		case <-a.stopCh:
			return
		}
	}
}

// flush flushes buffered records to outputs
func (a *LogAggregator) flush() {
	records := a.buffer.GetAndClear()
	if len(records) == 0 {
		return
	}

	// Write to outputs
	for _, output := range a.outputs {
		if err := a.writeToOutput(output, records); err != nil {
			// Log error but continue with other outputs
			fmt.Fprintf(os.Stderr, "Output handler failed: %v\n", err)
		}
	}

	// Export if configured
	if a.exporter != nil {
		a.exporter.Export(records)
	}

	// Update metrics
	a.metrics.mutex.Lock()
	a.metrics.BufferedRecords -= int64(len(records))
	a.metrics.LastFlushTime = time.Now()
	a.metrics.mutex.Unlock()
}

// writeToOutput writes records to an output with retry logic
func (a *LogAggregator) writeToOutput(output OutputHandler, records []*LogRecord) error {
	var lastErr error
	
	for i := 0; i < a.config.MaxRetries; i++ {
		if err := output.Write(records); err != nil {
			lastErr = err
			time.Sleep(a.config.RetryDelay)
			continue
		}
		return nil
	}
	
	return fmt.Errorf("failed after %d retries: %w", a.config.MaxRetries, lastErr)
}

// GetMetrics returns aggregator metrics
func (a *LogAggregator) GetMetrics() *AggregatorMetrics {
	a.metrics.mutex.RLock()
	defer a.metrics.mutex.RUnlock()

	// Return a copy to avoid concurrent access
	return &AggregatorMetrics{
		TotalRecords:     a.metrics.TotalRecords,
		BufferedRecords:  a.metrics.BufferedRecords,
		ExportedRecords:  a.metrics.ExportedRecords,
		FailedExports:    a.metrics.FailedExports,
		RotationCount:    a.metrics.RotationCount,
		LastFlushTime:    a.metrics.LastFlushTime,
		LastRotationTime: a.metrics.LastRotationTime,
	}
}

// LogBuffer implementation

// NewLogBuffer creates a new log buffer
func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{
		records: make([]LogRecord, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add adds a record to the buffer
func (b *LogBuffer) Add(record LogRecord) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.records = append(b.records, record)
	
	// Remove oldest records if buffer is full
	if len(b.records) > b.maxSize {
		copy(b.records, b.records[len(b.records)-b.maxSize:])
		b.records = b.records[:b.maxSize]
	}
}

// GetAndClear returns all records and clears the buffer
func (b *LogBuffer) GetAndClear() []*LogRecord {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if len(b.records) == 0 {
		return nil
	}

	// Create pointers to records
	result := make([]*LogRecord, len(b.records))
	for i := range b.records {
		result[i] = &b.records[i]
	}

	// Clear buffer
	b.records = b.records[:0]

	return result
}

// Size returns current buffer size
func (b *LogBuffer) Size() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	
	return len(b.records)
}

// FileOutputHandler writes logs to files

// FileOutputHandler implements OutputHandler for file output
type FileOutputHandler struct {
	file     *os.File
	encoder  RecordEncoder
	rotator  *LogRotator
	mutex    sync.RWMutex
}

// NewFileOutputHandler creates a new file output handler
func NewFileOutputHandler(filename string, encoder RecordEncoder, rotator *LogRotator) (*FileOutputHandler, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &FileOutputHandler{
		file:    file,
		encoder: encoder,
		rotator: rotator,
	}, nil
}

// Write implements OutputHandler interface
func (h *FileOutputHandler) Write(records []*LogRecord) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	for _, record := range records {
		data, err := h.encoder.Encode(record)
		if err != nil {
			return fmt.Errorf("failed to encode record: %w", err)
		}

		if _, err := h.file.Write(data); err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}

		// Check if rotation is needed
		if h.rotator != nil {
			if err := h.rotator.CheckAndRotate(h.file, int64(len(data))); err != nil {
				return fmt.Errorf("rotation failed: %w", err)
			}
		}
	}

	return h.file.Sync()
}

// Close implements OutputHandler interface
func (h *FileOutputHandler) Close() error {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	
	return h.file.Close()
}

// RecordEncoder encodes log records

// RecordEncoder interface for encoding log records
type RecordEncoder interface {
	Encode(record *LogRecord) ([]byte, error)
}

// JSONRecordEncoder encodes records as JSON
type JSONRecordEncoder struct{}

// Encode implements RecordEncoder interface
func (e *JSONRecordEncoder) Encode(record *LogRecord) ([]byte, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	
	// Add newline
	data = append(data, '\n')
	return data, nil
}

// TextRecordEncoder encodes records as text
type TextRecordEncoder struct{}

// Encode implements RecordEncoder interface
func (e *TextRecordEncoder) Encode(record *LogRecord) ([]byte, error) {
	line := fmt.Sprintf("[%s] %v %s",
		record.Timestamp.Format(time.RFC3339),
		record.Level,
		record.Message,
	)
	
	// Add attributes
	if len(record.Attrs) > 0 {
		attrs := make([]string, 0, len(record.Attrs))
		for k, v := range record.Attrs {
			attrs = append(attrs, fmt.Sprintf("%s=%v", k, v))
		}
		sort.Strings(attrs) // For consistent output
		line += " " + fmt.Sprintf("{%s}", fmt.Sprintf("%v", attrs))
	}
	
	line += "\n"
	return []byte(line), nil
}

// LogRotator implementation

// NewLogRotator creates a new log rotator
func NewLogRotator(config *RotationConfig) *LogRotator {
	return &LogRotator{
		config: config,
	}
}

// CheckAndRotate checks if rotation is needed and performs it
func (r *LogRotator) CheckAndRotate(file *os.File, bytesWritten int64) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.currentSize += bytesWritten

	// Check if rotation is needed
	if r.shouldRotate() {
		return r.rotate(file)
	}

	return nil
}

// shouldRotate checks if rotation is needed
func (r *LogRotator) shouldRotate() bool {
	if r.config.MaxSize > 0 && r.currentSize >= r.config.MaxSize {
		return true
	}

	// Add time-based rotation logic here if needed
	
	return false
}

// rotate performs log rotation
func (r *LogRotator) rotate(file *os.File) error {
	// Implementation would depend on specific rotation strategy
	// This is a simplified version
	
	fileName := file.Name()
	
	// Close current file
	file.Close()
	
	// Rename current file
	backupName := fmt.Sprintf("%s.%d", fileName, time.Now().Unix())
	if err := os.Rename(fileName, backupName); err != nil {
		return fmt.Errorf("failed to rename log file: %w", err)
	}
	
	// Compress if configured
	if r.config.Compress {
		// Compression logic would go here
	}
	
	// Clean up old backups
	r.cleanupOldBackups(filepath.Dir(fileName))
	
	// Reset current size
	r.currentSize = 0
	
	return nil
}

// cleanupOldBackups removes old backup files
func (r *LogRotator) cleanupOldBackups(dir string) {
	// Implementation would scan directory and remove old backups
	// based on MaxBackups and MaxAge configuration
}

// Close closes the rotator
func (r *LogRotator) Close() error {
	if r.currentFile != nil {
		return r.currentFile.Close()
	}
	return nil
}

// LogExporter implementation

// NewLogExporter creates a new log exporter
func NewLogExporter(config *ExportConfig) *LogExporter {
	return &LogExporter{
		config: config,
		buffer: make([]LogRecord, 0, config.BatchSize),
	}
}

// Export exports log records
func (e *LogExporter) Export(records []*LogRecord) error {
	if !e.config.Enabled || e.client == nil {
		return nil
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Add records to buffer
	for _, record := range records {
		e.buffer = append(e.buffer, *record)
	}

	// Export if batch size reached
	if len(e.buffer) >= e.config.BatchSize {
		return e.exportBatch()
	}

	return nil
}

// exportBatch exports a batch of records
func (e *LogExporter) exportBatch() error {
	if len(e.buffer) == 0 {
		return nil
	}

	err := e.client.Export(e.buffer)
	if err == nil {
		e.buffer = e.buffer[:0] // Clear buffer
	}

	return err
}

// Close closes the exporter
func (e *LogExporter) Close() error {
	// Export remaining records
	e.exportBatch()
	
	if e.client != nil {
		return e.client.Close()
	}
	return nil
}