package logger

import (
	"bytes"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		wantErr  bool
		checkFn  func(*testing.T, *slog.Logger)
	}{
		{
			name:    "valid info level",
			level:   "info",
			wantErr: false,
			checkFn: func(t *testing.T, logger *slog.Logger) {
				assert.NotNil(t, logger)
			},
		},
		{
			name:    "valid debug level",
			level:   "debug",
			wantErr: false,
			checkFn: func(t *testing.T, logger *slog.Logger) {
				assert.NotNil(t, logger)
			},
		},
		{
			name:    "valid warn level",
			level:   "warn",
			wantErr: false,
			checkFn: func(t *testing.T, logger *slog.Logger) {
				assert.NotNil(t, logger)
			},
		},
		{
			name:    "valid error level",
			level:   "error",
			wantErr: false,
			checkFn: func(t *testing.T, logger *slog.Logger) {
				assert.NotNil(t, logger)
			},
		},
		{
			name:    "invalid level",
			level:   "invalid",
			wantErr: true,
		},
		{
			name:    "empty level defaults to info",
			level:   "",
			wantErr: true, // empty string should fail parsing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New(tt.level)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, logger)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, logger)
				if tt.checkFn != nil {
					tt.checkFn(t, logger)
				}
			}
		})
	}
}

func TestNewWithWriter(t *testing.T) {
	var buf bytes.Buffer
	
	logger, err := NewWithWriter("info", &buf)
	require.NoError(t, err)
	require.NotNil(t, logger)
	
	// Test that logger writes to the buffer
	logger.Info("test message", "key", "value")
	
	output := buf.String()
	assert.Contains(t, output, "test message")
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "value")
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  slog.Level
		wantError bool
	}{
		{
			name:     "debug level",
			input:    "debug",
			expected: slog.LevelDebug,
		},
		{
			name:     "info level",
			input:    "info",
			expected: slog.LevelInfo,
		},
		{
			name:     "warn level",
			input:    "warn",
			expected: slog.LevelWarn,
		},
		{
			name:     "warning level",
			input:    "warning",
			expected: slog.LevelWarn,
		},
		{
			name:     "error level",
			input:    "error",
			expected: slog.LevelError,
		},
		{
			name:     "uppercase debug",
			input:    "DEBUG",
			expected: slog.LevelDebug,
		},
		{
			name:      "invalid level",
			input:     "invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, err := parseLogLevel(tt.input)
			
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, level)
			}
		})
	}
}

func TestIsProduction(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{
			name:     "production",
			envValue: "production",
			expected: true,
		},
		{
			name:     "prod",
			envValue: "prod",
			expected: true,
		},
		{
			name:     "PRODUCTION uppercase",
			envValue: "PRODUCTION",
			expected: true,
		},
		{
			name:     "development",
			envValue: "development",
			expected: false,
		},
		{
			name:     "empty",
			envValue: "",
			expected: false,
		},
	}

	// Save original env value
	originalEnv := os.Getenv("GO_ENV")
	defer os.Setenv("GO_ENV", originalEnv)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("GO_ENV", tt.envValue)
			result := isProduction()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWithComponent(t *testing.T) {
	var buf bytes.Buffer
	
	baseLogger, err := NewWithWriter("info", &buf)
	require.NoError(t, err)
	
	componentLogger := WithComponent(baseLogger, "test-component")
	componentLogger.Info("test message")
	
	output := buf.String()
	assert.Contains(t, output, "test-component")
	assert.Contains(t, output, "component")
}

func TestWithUser(t *testing.T) {
	var buf bytes.Buffer
	
	baseLogger, err := NewWithWriter("info", &buf)
	require.NoError(t, err)
	
	userLogger := WithUser(baseLogger, "user-123")
	userLogger.Info("test message")
	
	output := buf.String()
	assert.Contains(t, output, "user-123")
	assert.Contains(t, output, "user_id")
}

func TestWithTenant(t *testing.T) {
	var buf bytes.Buffer
	
	baseLogger, err := NewWithWriter("info", &buf)
	require.NoError(t, err)
	
	tenantLogger := WithTenant(baseLogger, "tenant-456")
	tenantLogger.Info("test message")
	
	output := buf.String()
	assert.Contains(t, output, "tenant-456")
	assert.Contains(t, output, "tenant_id")
}

func TestWithRequest(t *testing.T) {
	var buf bytes.Buffer
	
	baseLogger, err := NewWithWriter("info", &buf)
	require.NoError(t, err)
	
	requestLogger := WithRequest(baseLogger, "req-789", "GET", "/api/users")
	requestLogger.Info("test message")
	
	output := buf.String()
	assert.Contains(t, output, "req-789")
	assert.Contains(t, output, "GET")
	assert.Contains(t, output, "/api/users")
	assert.Contains(t, output, "request_id")
	assert.Contains(t, output, "method")
	assert.Contains(t, output, "path")
}

func TestLogError(t *testing.T) {
	var buf bytes.Buffer
	
	logger, err := NewWithWriter("info", &buf)
	require.NoError(t, err)
	
	testErr := assert.AnError
	LogError(logger, testErr, "test error occurred", "context", "test")
	
	output := buf.String()
	assert.Contains(t, output, "test error occurred")
	assert.Contains(t, output, "error")
	assert.Contains(t, output, "context")
	assert.Contains(t, output, "test")
}

func TestLogDuration(t *testing.T) {
	var buf bytes.Buffer
	
	logger, err := NewWithWriter("info", &buf)
	require.NoError(t, err)
	
	start := time.Now().Add(-100 * time.Millisecond) // Simulate 100ms ago
	LogDuration(logger, start, "test operation", "result", "success")
	
	output := buf.String()
	assert.Contains(t, output, "Operation completed")
	assert.Contains(t, output, "test operation")
	assert.Contains(t, output, "duration_ms")
	assert.Contains(t, output, "result")
	assert.Contains(t, output, "success")
}

func TestDatabaseLogger(t *testing.T) {
	var buf bytes.Buffer
	
	baseLogger, err := NewWithWriter("info", &buf)
	require.NoError(t, err)
	
	dbLogger := DatabaseLogger(baseLogger)
	dbLogger.Info("database operation")
	
	output := buf.String()
	assert.Contains(t, output, "database")
	assert.Contains(t, output, "component")
}

func TestKratosLogger(t *testing.T) {
	var buf bytes.Buffer
	
	baseLogger, err := NewWithWriter("info", &buf)
	require.NoError(t, err)
	
	kratosLogger := KratosLogger(baseLogger)
	kratosLogger.Info("kratos operation")
	
	output := buf.String()
	assert.Contains(t, output, "kratos")
	assert.Contains(t, output, "component")
}

// Test that different log levels work correctly
func TestLogLevels(t *testing.T) {
	tests := []struct {
		name        string
		logLevel    string
		logMessage  func(*slog.Logger)
		shouldShow  bool
	}{
		{
			name:     "debug message with debug level",
			logLevel: "debug",
			logMessage: func(l *slog.Logger) {
				l.Debug("debug message")
			},
			shouldShow: true,
		},
		{
			name:     "debug message with info level",
			logLevel: "info",
			logMessage: func(l *slog.Logger) {
				l.Debug("debug message")
			},
			shouldShow: false,
		},
		{
			name:     "info message with info level",
			logLevel: "info",
			logMessage: func(l *slog.Logger) {
				l.Info("info message")
			},
			shouldShow: true,
		},
		{
			name:     "warn message with error level",
			logLevel: "error",
			logMessage: func(l *slog.Logger) {
				l.Warn("warn message")
			},
			shouldShow: false,
		},
		{
			name:     "error message with error level",
			logLevel: "error",
			logMessage: func(l *slog.Logger) {
				l.Error("error message")
			},
			shouldShow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			
			logger, err := NewWithWriter(tt.logLevel, &buf)
			require.NoError(t, err)
			
			tt.logMessage(logger)
			
			output := buf.String()
			if tt.shouldShow {
				assert.NotEmpty(t, output)
			} else {
				assert.Empty(t, output)
			}
		})
	}
}