package logger

import (
	"log/slog"
)

// Logger is the global logger instance
var Logger *slog.Logger

// init sets up a no-op logger for tests to avoid nil-pointer panics when
// the application code uses logger.Logger before the main package configures
// it. Production code still overrides this value in main.go.
func init() {
	if Logger == nil {
		// Use UnifiedLogger for consistent Alt-backend compatible output
		unifiedLogger := NewUnifiedLogger("pre-processor")
		Logger = unifiedLogger.logger
	}
}

// InitGlobalLogger updates the global Logger with UnifiedLogger
func InitGlobalLogger(config *UnifiedLoggerConfig) {
	unifiedLogger := NewUnifiedLoggerWithLevel(config.ServiceName, config.Level)
	Logger = unifiedLogger.logger
}
