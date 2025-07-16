package system_port

import (
	"context"
	"time"
)

// SystemPort defines the interface for system operations
type SystemPort interface {
	// ExecuteCommand executes a system command with given arguments
	ExecuteCommand(ctx context.Context, command string, args ...string) (string, error)
	
	// ExecuteCommandWithTimeout executes a system command with timeout
	ExecuteCommandWithTimeout(ctx context.Context, timeout time.Duration, command string, args ...string) (string, error)
	
	// CheckCommandExists checks if a command exists in the system
	CheckCommandExists(command string) bool
	
	// Sleep pauses execution for the specified duration
	Sleep(duration time.Duration)
	
	// GetEnvironmentVariable gets an environment variable value
	GetEnvironmentVariable(key string) string
	
	// SetEnvironmentVariable sets an environment variable
	SetEnvironmentVariable(key, value string) error
}