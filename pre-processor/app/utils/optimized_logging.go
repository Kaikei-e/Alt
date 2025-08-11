package utils

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	logger "pre-processor/utils/logger"
)

// LogConfig holds logging configuration
type LogConfig struct {
	Level           string `env:"LOG_LEVEL" default:"info"`
	Format          string `env:"LOG_FORMAT" default:"json"`
	SamplingEnabled bool   `env:"LOG_SAMPLING_ENABLED" default:"true"`
	SamplingRate    int    `env:"LOG_SAMPLING_RATE" default:"100"`
}

// LoggerFactory manages optimized logger instances
type LoggerFactory struct {
	config  LogConfig
	loggers map[string]*OptimizedLogger
	mu      sync.RWMutex
}

// NewLoggerFactory creates a new logger factory
func NewLoggerFactory(config LogConfig) *LoggerFactory {
	return &LoggerFactory{
		config:  config,
		loggers: make(map[string]*OptimizedLogger),
	}
}

// GetLogger returns a logger instance for the specified component
func (f *LoggerFactory) GetLogger(component string) *OptimizedLogger {
	f.mu.RLock()
	logger, exists := f.loggers[component]
	f.mu.RUnlock()

	if !exists {
		f.mu.Lock()
		// Double-check after acquiring write lock
		if logger, exists = f.loggers[component]; !exists {
			logger = NewOptimizedLogger(component, f.config)
			f.loggers[component] = logger
		}
		f.mu.Unlock()
	}

	return logger
}

// OptimizedLogger provides optimized logging with context support
type OptimizedLogger struct {
	component     string
	config        LogConfig
	baseLogger    *slog.Logger
	contextFields map[string]interface{}
	mu            sync.RWMutex
}

// NewOptimizedLogger creates a new optimized logger
func NewOptimizedLogger(component string, config LogConfig) *OptimizedLogger {
	return &OptimizedLogger{
		component:     component,
		config:        config,
		baseLogger:    logger.Logger,
		contextFields: make(map[string]interface{}),
	}
}

// WithContext creates a logger with context values
func (l *OptimizedLogger) WithContext(ctx context.Context) *OptimizedLogger {
	l.mu.Lock()
	defer l.mu.Unlock()

	contextFields := make(map[string]interface{})

	// Copy existing context fields
	for k, v := range l.contextFields {
		contextFields[k] = v
	}

	// Add context values
	if requestID := ctx.Value("request_id"); requestID != nil {
		contextFields["request_id"] = requestID
	}
	if userID := ctx.Value("user_id"); userID != nil {
		contextFields["user_id"] = userID
	}
	if traceID := ctx.Value("trace_id"); traceID != nil {
		contextFields["trace_id"] = traceID
	}

	return &OptimizedLogger{
		component:     l.component,
		config:        l.config,
		baseLogger:    l.baseLogger,
		contextFields: contextFields,
	}
}

// Info logs an info level message
func (l *OptimizedLogger) Info(msg string, args ...interface{}) {
	l.log("info", msg, args...)
}

// Error logs an error level message
func (l *OptimizedLogger) Error(msg string, args ...interface{}) {
	l.log("error", msg, args...)
}

// Debug logs a debug level message
func (l *OptimizedLogger) Debug(msg string, args ...interface{}) {
	l.log("debug", msg, args...)
}

// Warn logs a warning level message
func (l *OptimizedLogger) Warn(msg string, args ...interface{}) {
	l.log("warn", msg, args...)
}

// log is the internal logging method
func (l *OptimizedLogger) log(level string, msg string, args ...interface{}) {
	if l.baseLogger == nil {
		return
	}

	// Prepare arguments with context fields while guarding against overflow
	const maxInt = int(^uint(0) >> 1)
	cap := uint64(len(args)) + uint64(len(l.contextFields))*2 + 2
	if cap > uint64(maxInt) {
		cap = uint64(maxInt)
	}
	allArgs := make([]interface{}, 0, int(cap))
	allArgs = append(allArgs, "component", l.component)

	// Add context fields
	l.mu.RLock()
	for k, v := range l.contextFields {
		allArgs = append(allArgs, k, v)
	}
	l.mu.RUnlock()

	// Add provided arguments
	allArgs = append(allArgs, args...)

	// Log based on level
	switch level {
	case "info":
		l.baseLogger.Info(msg, allArgs...)
	case "error":
		l.baseLogger.Error(msg, allArgs...)
	case "debug":
		l.baseLogger.Debug(msg, allArgs...)
	case "warn":
		l.baseLogger.Warn(msg, allArgs...)
	}
}

// SamplingLogger implements log sampling to reduce I/O overhead
type SamplingLogger struct {
	logger       *OptimizedLogger
	samplingRate int
	counter      uint64
}

// NewSamplingLogger creates a sampling logger
func NewSamplingLogger(logger *OptimizedLogger, samplingRate int) *SamplingLogger {
	return &SamplingLogger{
		logger:       logger,
		samplingRate: samplingRate,
	}
}

// LogSampled logs a message based on sampling rate
func (s *SamplingLogger) LogSampled(level string, msg string, args ...interface{}) {
	count := atomic.AddUint64(&s.counter, 1)

	// Sample based on rate with safe conversion
	if s.samplingRate <= 0 {
		// If sampling rate is invalid, don't log
		return
	}

	// Safe conversion of int to uint64
	var samplingRate uint64
	if s.samplingRate > 0 {
		samplingRate = uint64(s.samplingRate)
	} else {
		// If negative, skip sampling
		return
	}

	if count%samplingRate == 0 {
		// Add sampling information
		sampledArgs := make([]interface{}, 0, len(args)+2)
		sampledArgs = append(sampledArgs, "sampled_count", count)
		sampledArgs = append(sampledArgs, args...)

		switch level {
		case "info":
			s.logger.Info(msg, sampledArgs...)
		case "error":
			s.logger.Error(msg, sampledArgs...)
		case "debug":
			s.logger.Debug(msg, sampledArgs...)
		case "warn":
			s.logger.Warn(msg, sampledArgs...)
		}
	}
}

// LogCondition represents a logging condition function
type LogCondition func(level string, msg string) bool

// ConditionalLogger implements conditional logging
type ConditionalLogger struct {
	logger     *OptimizedLogger
	conditions []LogCondition
}

// NewConditionalLogger creates a conditional logger
func NewConditionalLogger(logger *OptimizedLogger, conditions []LogCondition) *ConditionalLogger {
	return &ConditionalLogger{
		logger:     logger,
		conditions: conditions,
	}
}

// ShouldLog checks if a message should be logged based on conditions
func (c *ConditionalLogger) ShouldLog(level string, msg string) bool {
	for _, condition := range c.conditions {
		if !condition(level, msg) {
			return false
		}
	}
	return true
}

// Info logs an info message if conditions are met
func (c *ConditionalLogger) Info(msg string, args ...interface{}) {
	if c.ShouldLog("info", msg) {
		c.logger.Info(msg, args...)
	}
}

// Error logs an error message if conditions are met
func (c *ConditionalLogger) Error(msg string, args ...interface{}) {
	if c.ShouldLog("error", msg) {
		c.logger.Error(msg, args...)
	}
}

// LogFieldCache caches computed log field values
type LogFieldCache struct {
	fields map[string]interface{}
	mu     sync.RWMutex
}

// NewLogFieldCache creates a new log field cache
func NewLogFieldCache() *LogFieldCache {
	return &LogFieldCache{
		fields: make(map[string]interface{}),
	}
}

// GetOrCompute gets a cached value or computes it if not present
func (c *LogFieldCache) GetOrCompute(key string, compute func() interface{}) interface{} {
	c.mu.RLock()
	if val, ok := c.fields[key]; ok {
		c.mu.RUnlock()
		return val
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if val, ok := c.fields[key]; ok {
		return val
	}

	val := compute()
	c.fields[key] = val
	return val
}

// Clear removes all cached values
func (c *LogFieldCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fields = make(map[string]interface{})
}

// LoadLogConfigFromEnv loads logging configuration from environment variables
func LoadLogConfigFromEnv() LogConfig {
	return LogConfig{
		Level:           getEnvOrDefault("LOG_LEVEL", "info"),
		Format:          getEnvOrDefault("LOG_FORMAT", "json"),
		SamplingEnabled: getEnvOrDefault("LOG_SAMPLING_ENABLED", "true") == "true",
		SamplingRate:    parseEnvInt("LOG_SAMPLING_RATE", 100),
	}
}

// parseEnvInt parses an environment variable as an integer
func parseEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := parseInt(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultValue
}

// parseInt is a simple integer parser
func parseInt(s string) (int, error) {
	result := 0
	for _, char := range s {
		if char < '0' || char > '9' {
			return 0, fmt.Errorf("invalid integer: %s", s)
		}
		result = result*10 + int(char-'0')
	}
	return result, nil
}

// Global optimized logger instance
var (
	globalLoggerFactory *LoggerFactory
	factoryOnce         sync.Once
)

// GetGlobalLoggerFactory returns the global logger factory instance
func GetGlobalLoggerFactory() *LoggerFactory {
	factoryOnce.Do(func() {
		config := LoadLogConfigFromEnv()
		globalLoggerFactory = NewLoggerFactory(config)
	})
	return globalLoggerFactory
}

// GetOptimizedLogger is a convenience function to get an optimized logger
func GetOptimizedLogger(component string) *OptimizedLogger {
	factory := GetGlobalLoggerFactory()
	return factory.GetLogger(component)
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
