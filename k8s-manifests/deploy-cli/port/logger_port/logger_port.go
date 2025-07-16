package logger_port

// LoggerPort defines the interface for logging operations
type LoggerPort interface {
	// Info logs an info message
	Info(msg string, args ...interface{})
	
	// Error logs an error message
	Error(msg string, args ...interface{})
	
	// Warn logs a warning message
	Warn(msg string, args ...interface{})
	
	// Debug logs a debug message
	Debug(msg string, args ...interface{})
	
	// InfoWithContext logs an info message with context
	InfoWithContext(msg string, context map[string]interface{})
	
	// ErrorWithContext logs an error message with context
	ErrorWithContext(msg string, context map[string]interface{})
	
	// WarnWithContext logs a warning message with context
	WarnWithContext(msg string, context map[string]interface{})
	
	// DebugWithContext logs a debug message with context
	DebugWithContext(msg string, context map[string]interface{})
	
	// WithField adds a field to the logger context
	WithField(key string, value interface{}) LoggerPort
	
	// WithFields adds multiple fields to the logger context
	WithFields(fields map[string]interface{}) LoggerPort
}