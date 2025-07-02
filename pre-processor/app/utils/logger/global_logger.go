package logger

import (
	"os"
)

// init sets up a no-op logger for tests to avoid nil-pointer panics when
// the application code uses logger.Logger before the main package configures
// it. Production code still overrides this value in main.go.
func init() {
	if Logger == nil {
		// Use UnifiedLogger for consistent Alt-backend compatible output
		unifiedLogger := NewUnifiedLogger(os.Stderr, "pre-processor")
		Logger = unifiedLogger.logger
	}
}

// InitGlobalLogger updates the global Logger with UnifiedLogger
func InitGlobalLogger(config *LoggerConfig) {
	unifiedLogger := NewUnifiedLoggerWithLevel(os.Stdout, config.ServiceName, config.Level)
	Logger = unifiedLogger.logger
}
