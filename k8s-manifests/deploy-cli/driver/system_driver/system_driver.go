package system_driver

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
	
	"deploy-cli/port/system_port"
)

// SystemDriver implements system operations
type SystemDriver struct{}

// Ensure SystemDriver implements SystemPort interface
var _ system_port.SystemPort = (*SystemDriver)(nil)

// NewSystemDriver creates a new system driver
func NewSystemDriver() *SystemDriver {
	return &SystemDriver{}
}

// ExecuteCommand executes a system command with given arguments
func (s *SystemDriver) ExecuteCommand(ctx context.Context, command string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// ExecuteCommandWithTimeout executes a system command with timeout
func (s *SystemDriver) ExecuteCommandWithTimeout(ctx context.Context, timeout time.Duration, command string, args ...string) (string, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	return s.ExecuteCommand(ctxWithTimeout, command, args...)
}

// CheckCommandExists checks if a command exists in the system
func (s *SystemDriver) CheckCommandExists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// Sleep pauses execution for the specified duration
func (s *SystemDriver) Sleep(duration time.Duration) {
	time.Sleep(duration)
}

// GetEnvironmentVariable gets an environment variable value
func (s *SystemDriver) GetEnvironmentVariable(key string) string {
	return os.Getenv(key)
}

// SetEnvironmentVariable sets an environment variable
func (s *SystemDriver) SetEnvironmentVariable(key, value string) error {
	return os.Setenv(key, value)
}

// ExecuteScript executes a shell script
func (s *SystemDriver) ExecuteScript(ctx context.Context, scriptPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", append([]string{scriptPath}, args...)...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// GetCurrentDirectory returns the current working directory
func (s *SystemDriver) GetCurrentDirectory() (string, error) {
	return os.Getwd()
}

// ChangeDirectory changes the current working directory
func (s *SystemDriver) ChangeDirectory(path string) error {
	return os.Chdir(path)
}

// GetSystemInfo returns basic system information
func (s *SystemDriver) GetSystemInfo(ctx context.Context) (map[string]string, error) {
	info := make(map[string]string)
	
	// Get OS type
	if output, err := s.ExecuteCommand(ctx, "uname", "-s"); err == nil {
		info["os"] = strings.TrimSpace(output)
	}
	
	// Get kernel version
	if output, err := s.ExecuteCommand(ctx, "uname", "-r"); err == nil {
		info["kernel"] = strings.TrimSpace(output)
	}
	
	// Get architecture
	if output, err := s.ExecuteCommand(ctx, "uname", "-m"); err == nil {
		info["architecture"] = strings.TrimSpace(output)
	}
	
	return info, nil
}