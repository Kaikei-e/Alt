// PHASE R3: Shared infrastructure for all commands
package shared

import (
	"fmt"

	"github.com/spf13/cobra"

	"deploy-cli/driver/filesystem_driver"
	"deploy-cli/driver/helm_driver"
	"deploy-cli/driver/kubectl_driver"
	"deploy-cli/driver/system_driver"
	"deploy-cli/port/logger_port"
	"deploy-cli/utils/logger"
)

// CommandShared provides shared infrastructure for all commands
type CommandShared struct {
	// Logging
	Logger     *logger.Logger
	LoggerPort logger_port.LoggerPort

	// Drivers
	SystemDriver     *system_driver.SystemDriver
	HelmDriver       *helm_driver.HelmDriver
	KubectlDriver    *kubectl_driver.KubectlDriver
	FilesystemDriver *filesystem_driver.FileSystemDriver

	// Factories
	DeploymentUsecaseFactory *DeploymentUsecaseFactory
	SecretUsecaseFactory     *SecretUsecaseFactory

	// Utilities
	EnvironmentParser *EnvironmentParser
	FlagParser        *FlagParser
	OutputFormatter   *OutputFormatter
	ValidationHelper  *ValidationHelper

	// Configuration
	Config *SharedConfig
}

// SharedConfig holds shared configuration for all commands
type SharedConfig struct {
	// Global settings
	DefaultTimeout    string
	DefaultChartsDir  string
	LogLevel          string
	OutputFormat      string

	// Feature flags
	EnableAutoFix     bool
	EnableDiagnostics bool
	EnableMetrics     bool
}

// NewCommandShared creates a new shared command infrastructure
func NewCommandShared(logger *logger.Logger) *CommandShared {
	shared := &CommandShared{
		Logger: logger,
		Config: NewDefaultSharedConfig(),
	}

	// Initialize components
	shared.initializeDrivers()
	shared.initializeLoggerPort()
	shared.initializeFactories()
	shared.initializeUtilities()

	return shared
}

// PersistentPreRunE provides common pre-run logic for all commands
func (s *CommandShared) PersistentPreRunE(cmd *cobra.Command, args []string) error {
	// Update log level if specified
	if logLevel, err := cmd.Flags().GetString("log-level"); err == nil && logLevel != "" {
		s.Config.LogLevel = logLevel
		// Note: slog.Logger doesn't support runtime level changes
		// Consider creating a new logger instance if dynamic level change is needed
	}

	// Update output format if specified
	if outputFormat, err := cmd.Flags().GetString("output"); err == nil && outputFormat != "" {
		s.Config.OutputFormat = outputFormat
	}

	s.Logger.DebugWithContext("shared pre-run completed", map[string]interface{}{
		"command":       cmd.Name(),
		"log_level":     s.Config.LogLevel,
		"output_format": s.Config.OutputFormat,
	})

	return nil
}

// AddGlobalFlags adds global flags that are available to all commands
func (s *CommandShared) AddGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("log-level", s.Config.LogLevel,
		"Set the log level (debug, info, warn, error)")
	cmd.PersistentFlags().StringP("output", "o", s.Config.OutputFormat,
		"Output format (text, json, yaml)")
	cmd.PersistentFlags().Bool("no-color", false,
		"Disable colored output")
	cmd.PersistentFlags().Bool("enable-metrics", s.Config.EnableMetrics,
		"Enable performance metrics collection")
}

// ValidateGlobalFlags validates global flags for consistency
func (s *CommandShared) ValidateGlobalFlags(cmd *cobra.Command) error {
	// Validate log level
	logLevel, _ := cmd.Flags().GetString("log-level")
	if !s.isValidLogLevel(logLevel) {
		return fmt.Errorf("invalid log level '%s'. Valid levels: debug, info, warn, error", logLevel)
	}

	// Validate output format
	outputFormat, _ := cmd.Flags().GetString("output")
	if !s.isValidOutputFormat(outputFormat) {
		return fmt.Errorf("invalid output format '%s'. Valid formats: text, json, yaml", outputFormat)
	}

	return nil
}

// Private initialization methods

// initializeDrivers initializes all driver dependencies
func (s *CommandShared) initializeDrivers() {
	s.SystemDriver = system_driver.NewSystemDriver()
	s.HelmDriver = helm_driver.NewHelmDriver()
	s.KubectlDriver = kubectl_driver.NewKubectlDriver()
	s.FilesystemDriver = filesystem_driver.NewFileSystemDriver()
}

// initializeLoggerPort initializes the logger port adapter
func (s *CommandShared) initializeLoggerPort() {
	s.LoggerPort = NewLoggerPortAdapter(s.Logger)
}

// initializeFactories initializes usecase factories
func (s *CommandShared) initializeFactories() {
	s.DeploymentUsecaseFactory = NewDeploymentUsecaseFactory(s)
	s.SecretUsecaseFactory = NewSecretUsecaseFactory(s)
}

// initializeUtilities initializes shared utility components
func (s *CommandShared) initializeUtilities() {
	s.EnvironmentParser = NewEnvironmentParser(s)
	s.FlagParser = NewFlagParser(s)
	s.OutputFormatter = NewOutputFormatter(s)
	s.ValidationHelper = NewValidationHelper(s)
}

// Validation helper methods

// isValidLogLevel checks if the log level is valid
func (s *CommandShared) isValidLogLevel(level string) bool {
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	return validLevels[level]
}

// isValidOutputFormat checks if the output format is valid
func (s *CommandShared) isValidOutputFormat(format string) bool {
	validFormats := map[string]bool{
		"text": true,
		"json": true,
		"yaml": true,
	}
	return validFormats[format]
}

// NewDefaultSharedConfig creates a default shared configuration
func NewDefaultSharedConfig() *SharedConfig {
	return &SharedConfig{
		DefaultTimeout:    "300s",
		DefaultChartsDir:  "../charts",
		LogLevel:          "info",
		OutputFormat:      "text",
		EnableAutoFix:     false,
		EnableDiagnostics: false,
		EnableMetrics:     false,
	}
}

// LoggerPortAdapter adapts the logger to the logger port interface
type LoggerPortAdapter struct {
	logger *logger.Logger
}

// NewLoggerPortAdapter creates a new logger port adapter
func NewLoggerPortAdapter(logger *logger.Logger) logger_port.LoggerPort {
	return &LoggerPortAdapter{logger: logger}
}

// Info logs an info message
func (l *LoggerPortAdapter) Info(msg string, args ...interface{}) {
	l.logger.InfoWithContext(msg, args...)
}

// Error logs an error message
func (l *LoggerPortAdapter) Error(msg string, args ...interface{}) {
	l.logger.ErrorWithContext(msg, args...)
}

// Warn logs a warning message
func (l *LoggerPortAdapter) Warn(msg string, args ...interface{}) {
	l.logger.WarnWithContext(msg, args...)
}

// Debug logs a debug message
func (l *LoggerPortAdapter) Debug(msg string, args ...interface{}) {
	l.logger.DebugWithContext(msg, args...)
}

// InfoWithContext logs an info message with context
func (l *LoggerPortAdapter) InfoWithContext(msg string, context map[string]interface{}) {
	args := make([]interface{}, 0, len(context)*2)
	for k, v := range context {
		args = append(args, k, v)
	}
	l.logger.InfoWithContext(msg, args...)
}

// ErrorWithContext logs an error message with context
func (l *LoggerPortAdapter) ErrorWithContext(msg string, context map[string]interface{}) {
	args := make([]interface{}, 0, len(context)*2)
	for k, v := range context {
		args = append(args, k, v)
	}
	l.logger.ErrorWithContext(msg, args...)
}

// WarnWithContext logs a warning message with context
func (l *LoggerPortAdapter) WarnWithContext(msg string, context map[string]interface{}) {
	args := make([]interface{}, 0, len(context)*2)
	for k, v := range context {
		args = append(args, k, v)
	}
	l.logger.WarnWithContext(msg, args...)
}

// DebugWithContext logs a debug message with context
func (l *LoggerPortAdapter) DebugWithContext(msg string, context map[string]interface{}) {
	args := make([]interface{}, 0, len(context)*2)
	for k, v := range context {
		args = append(args, k, v)
	}
	l.logger.DebugWithContext(msg, args...)
}

// WithField adds a field to the logger context
func (l *LoggerPortAdapter) WithField(key string, value interface{}) logger_port.LoggerPort {
	return &LoggerPortAdapter{logger: l.logger.WithContext(key, value)}
}

// WithFields adds multiple fields to the logger context
func (l *LoggerPortAdapter) WithFields(fields map[string]interface{}) logger_port.LoggerPort {
	newLogger := l.logger
	for key, value := range fields {
		newLogger = newLogger.WithContext(key, value)
	}
	return &LoggerPortAdapter{logger: newLogger}
}