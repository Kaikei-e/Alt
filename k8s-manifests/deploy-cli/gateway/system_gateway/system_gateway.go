package system_gateway

import (
	"context"
	"fmt"
	"time"
	
	"deploy-cli/port/system_port"
	"deploy-cli/port/logger_port"
)

// SystemGateway acts as anti-corruption layer for system operations
type SystemGateway struct {
	systemPort system_port.SystemPort
	logger     logger_port.LoggerPort
}

// NewSystemGateway creates a new system gateway
func NewSystemGateway(systemPort system_port.SystemPort, logger logger_port.LoggerPort) *SystemGateway {
	return &SystemGateway{
		systemPort: systemPort,
		logger:     logger,
	}
}

// ExecuteCommand executes a system command with logging and error handling
func (g *SystemGateway) ExecuteCommand(ctx context.Context, command string, args ...string) (string, error) {
	g.logger.InfoWithContext("executing command", map[string]interface{}{
		"command": command,
		"args":    args,
	})
	
	start := time.Now()
	output, err := g.systemPort.ExecuteCommand(ctx, command, args...)
	duration := time.Since(start)
	
	if err != nil {
		g.logger.ErrorWithContext("command execution failed", map[string]interface{}{
			"command":  command,
			"args":     args,
			"error":    err.Error(),
			"duration": duration,
			"output":   output,
		})
		return output, fmt.Errorf("command execution failed: %w", err)
	}
	
	g.logger.InfoWithContext("command executed successfully", map[string]interface{}{
		"command":  command,
		"args":     args,
		"duration": duration,
	})
	
	return output, nil
}

// ExecuteCommandWithTimeout executes a system command with timeout
func (g *SystemGateway) ExecuteCommandWithTimeout(ctx context.Context, timeout time.Duration, command string, args ...string) (string, error) {
	g.logger.InfoWithContext("executing command with timeout", map[string]interface{}{
		"command": command,
		"args":    args,
		"timeout": timeout,
	})
	
	start := time.Now()
	output, err := g.systemPort.ExecuteCommandWithTimeout(ctx, timeout, command, args...)
	duration := time.Since(start)
	
	if err != nil {
		g.logger.ErrorWithContext("command execution with timeout failed", map[string]interface{}{
			"command":  command,
			"args":     args,
			"timeout":  timeout,
			"error":    err.Error(),
			"duration": duration,
			"output":   output,
		})
		return output, fmt.Errorf("command execution with timeout failed: %w", err)
	}
	
	g.logger.InfoWithContext("command executed successfully with timeout", map[string]interface{}{
		"command":  command,
		"args":     args,
		"timeout":  timeout,
		"duration": duration,
	})
	
	return output, nil
}

// CheckCommandExists checks if a command exists in the system
func (g *SystemGateway) CheckCommandExists(command string) bool {
	g.logger.DebugWithContext("checking command existence", map[string]interface{}{
		"command": command,
	})
	
	exists := g.systemPort.CheckCommandExists(command)
	
	g.logger.DebugWithContext("command existence check result", map[string]interface{}{
		"command": command,
		"exists":  exists,
	})
	
	return exists
}

// Sleep pauses execution for the specified duration
func (g *SystemGateway) Sleep(duration time.Duration) {
	g.logger.DebugWithContext("sleeping", map[string]interface{}{
		"duration": duration,
	})
	
	g.systemPort.Sleep(duration)
}

// GetEnvironmentVariable gets an environment variable value
func (g *SystemGateway) GetEnvironmentVariable(key string) string {
	g.logger.DebugWithContext("getting environment variable", map[string]interface{}{
		"key": key,
	})
	
	value := g.systemPort.GetEnvironmentVariable(key)
	
	g.logger.DebugWithContext("environment variable retrieved", map[string]interface{}{
		"key":   key,
		"value": value,
	})
	
	return value
}

// SetEnvironmentVariable sets an environment variable
func (g *SystemGateway) SetEnvironmentVariable(key, value string) error {
	g.logger.InfoWithContext("setting environment variable", map[string]interface{}{
		"key":   key,
		"value": value,
	})
	
	err := g.systemPort.SetEnvironmentVariable(key, value)
	if err != nil {
		g.logger.ErrorWithContext("failed to set environment variable", map[string]interface{}{
			"key":   key,
			"value": value,
			"error": err.Error(),
		})
		return fmt.Errorf("failed to set environment variable: %w", err)
	}
	
	g.logger.InfoWithContext("environment variable set successfully", map[string]interface{}{
		"key":   key,
		"value": value,
	})
	
	return nil
}

// ValidateRequiredCommands validates that required commands are available
func (g *SystemGateway) ValidateRequiredCommands(commands []string) error {
	g.logger.InfoWithContext("validating required commands", map[string]interface{}{
		"commands": commands,
	})
	
	var missingCommands []string
	for _, command := range commands {
		if !g.CheckCommandExists(command) {
			missingCommands = append(missingCommands, command)
		}
	}
	
	if len(missingCommands) > 0 {
		g.logger.ErrorWithContext("missing required commands", map[string]interface{}{
			"missing_commands": missingCommands,
		})
		return fmt.Errorf("missing required commands: %v", missingCommands)
	}
	
	g.logger.InfoWithContext("all required commands are available", map[string]interface{}{
		"commands": commands,
	})
	
	return nil
}

// ExecuteCommandSafely executes a command with error recovery
func (g *SystemGateway) ExecuteCommandSafely(ctx context.Context, command string, args ...string) (string, error) {
	// First check if command exists
	if !g.CheckCommandExists(command) {
		return "", fmt.Errorf("command not found: %s", command)
	}
	
	// Execute with retry logic
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		output, err := g.ExecuteCommand(ctx, command, args...)
		if err == nil {
			return output, nil
		}
		
		if i < maxRetries-1 {
			g.logger.WarnWithContext("command execution failed, retrying", map[string]interface{}{
				"command": command,
				"args":    args,
				"attempt": i + 1,
				"error":   err.Error(),
			})
			time.Sleep(time.Second * time.Duration(i+1))
		}
	}
	
	return "", fmt.Errorf("command execution failed after %d retries", maxRetries)
}

// GetSystemInfo returns system information
func (g *SystemGateway) GetSystemInfo(ctx context.Context) (map[string]string, error) {
	g.logger.InfoWithContext("getting system information", map[string]interface{}{})
	
	info := make(map[string]string)
	
	// Get OS information
	if output, err := g.ExecuteCommand(ctx, "uname", "-s"); err == nil {
		info["os"] = output
	}
	
	// Get kernel version
	if output, err := g.ExecuteCommand(ctx, "uname", "-r"); err == nil {
		info["kernel"] = output
	}
	
	// Get architecture
	if output, err := g.ExecuteCommand(ctx, "uname", "-m"); err == nil {
		info["architecture"] = output
	}
	
	g.logger.InfoWithContext("system information retrieved", map[string]interface{}{
		"info": info,
	})
	
	return info, nil
}