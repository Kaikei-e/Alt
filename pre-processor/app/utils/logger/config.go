// ABOUTME: This file provides simplified logger configuration for unified logger
// ABOUTME: Removes USE_RASK_LOGGER complexity and standardizes on slog
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// UnifiedLoggerConfig represents simplified logger configuration for unified logger
type UnifiedLoggerConfig struct {
	Level       string `env:"LOG_LEVEL" default:"info"`
	ServiceName string `env:"SERVICE_NAME" default:"pre-processor"`
	// Note: USE_RASK_LOGGER removed - unified on slog
}

// LoadUnifiedLoggerConfigFromEnv loads configuration from environment variables
func LoadUnifiedLoggerConfigFromEnv() *UnifiedLoggerConfig {
	return &UnifiedLoggerConfig{
		Level:       getEnvOrDefaultUnified("LOG_LEVEL", "info"),
		ServiceName: getEnvOrDefaultUnified("SERVICE_NAME", "pre-processor"),
	}
}

// InitializeUnifiedLogger creates a UnifiedLogger based on configuration
func InitializeUnifiedLogger(config *UnifiedLoggerConfig) *UnifiedLogger {
	return NewUnifiedLoggerWithLevel(os.Stdout, config.ServiceName, config.Level)
}

// NewUnifiedLoggerWithLevel creates a UnifiedLogger with specific log level
func NewUnifiedLoggerWithLevel(output io.Writer, serviceName, level string) *UnifiedLogger {
	// Parse log level string to slog.Level
	var slogLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo // Default to info level
	}

	// Configure Alt-backend compatible JSON handler with specified level
	options := &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Transform attributes to match Alt-backend format exactly
			switch a.Key {
			case slog.TimeKey:
				// Keep "time" field name like Alt-backend
				return slog.Attr{Key: "time", Value: a.Value}
			case slog.LevelKey:
				// Convert to "level" and lowercase for rask-log-forwarder compatibility
				if level, ok := a.Value.Any().(slog.Level); ok {
					return slog.Attr{Key: "level", Value: slog.StringValue(strings.ToLower(level.String()))}
				}
				return a
			case slog.MessageKey:
				// Convert to "msg" field name like Alt-backend
				return slog.Attr{Key: "msg", Value: a.Value}
			default:
				return a
			}
		},
	}

	handler := slog.NewJSONHandler(output, options)

	// Pre-configure with service name and version like Alt-backend
	logger := slog.New(handler).With("service", serviceName, "version", "1.0.0")

	return &UnifiedLogger{
		logger:      logger,
		serviceName: serviceName,
	}
}

// getEnvOrDefaultUnified returns environment variable value or default (unified logger specific)
func getEnvOrDefaultUnified(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
