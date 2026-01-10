// ABOUTME: This file provides simplified logger configuration for unified logger
// ABOUTME: Removes USE_RASK_LOGGER complexity and standardizes on slog
package logger

import (
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
	return NewUnifiedLoggerWithLevel(config.ServiceName, config.Level)
}

// NewUnifiedLoggerWithLevel creates a UnifiedLogger with specific log level
func NewUnifiedLoggerWithLevel(serviceName, level string) *UnifiedLogger {
	return NewUnifiedLoggerWithOTel(serviceName, level, false)
}

// NewUnifiedLoggerWithOTel creates a UnifiedLogger with optional OTel support
func NewUnifiedLoggerWithOTel(serviceName, level string, enableOTel bool) *UnifiedLogger {
	// Parse log level string to slog.Level
	slogLevel := parseSlogLevel(level)

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

	stdoutHandler := slog.NewJSONHandler(os.Stdout, options)

	var handler slog.Handler
	if enableOTel {
		// Use MultiHandler for JSON + OTel
		handler = NewMultiHandler(stdoutHandler, slogLevel)
	} else {
		handler = stdoutHandler
	}

	// Pre-configure with service name and version like Alt-backend
	logger := slog.New(handler).With("service", serviceName, "version", "1.0.0")

	return &UnifiedLogger{
		logger:      logger,
		serviceName: serviceName,
	}
}

// parseSlogLevel converts a string level to slog.Level
func parseSlogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo // Default to info level
	}
}

// getEnvOrDefaultUnified returns environment variable value or default (unified logger specific)
func getEnvOrDefaultUnified(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
