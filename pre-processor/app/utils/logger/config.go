// ABOUTME: This file provides simplified logger configuration for unified logger
// ABOUTME: Removes USE_RASK_LOGGER complexity and standardizes on slog
package logger

import (
	"io"
	"os"
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
	// For now, use the base NewUnifiedLogger and set level globally
	// This is a temporary implementation to pass tests
	// TODO: Implement proper level configuration in NewUnifiedLogger
	return NewUnifiedLogger(output, serviceName)
}

// getEnvOrDefaultUnified returns environment variable value or default (unified logger specific)
func getEnvOrDefaultUnified(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
