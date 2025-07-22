// PHASE R3: Shared flag parsing utilities
package shared

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// FlagParser provides shared flag parsing functionality
type FlagParser struct {
	shared *CommandShared
}

// NewFlagParser creates a new flag parser
func NewFlagParser(shared *CommandShared) *FlagParser {
	return &FlagParser{
		shared: shared,
	}
}

// ParseGlobalFlags parses common global flags
func (f *FlagParser) ParseGlobalFlags(cmd *cobra.Command) (*GlobalFlags, error) {
	flags := &GlobalFlags{}
	var err error

	// Parse common global flags
	if flags.DryRun, err = cmd.Flags().GetBool("dry-run"); err != nil {
		return nil, fmt.Errorf("failed to parse dry-run flag: %w", err)
	}
	if flags.Force, err = cmd.Flags().GetBool("force"); err != nil {
		return nil, fmt.Errorf("failed to parse force flag: %w", err)
	}
	if flags.Verbose, err = cmd.Flags().GetBool("verbose"); err != nil {
		return nil, fmt.Errorf("failed to parse verbose flag: %w", err)
	}
	if flags.Timeout, err = cmd.Flags().GetDuration("timeout"); err != nil {
		return nil, fmt.Errorf("failed to parse timeout flag: %w", err)
	}
	if flags.Output, err = cmd.Flags().GetString("output"); err != nil {
		return nil, fmt.Errorf("failed to parse output flag: %w", err)
	}
	if flags.LogLevel, err = cmd.Flags().GetString("log-level"); err != nil {
		return nil, fmt.Errorf("failed to parse log-level flag: %w", err)
	}
	if flags.NoColor, err = cmd.Flags().GetBool("no-color"); err != nil {
		return nil, fmt.Errorf("failed to parse no-color flag: %w", err)
	}

	// Parse optional auto-fix flag if present
	if cmd.Flags().Lookup("auto-fix") != nil {
		if flags.AutoFix, err = cmd.Flags().GetBool("auto-fix"); err != nil {
			return nil, fmt.Errorf("failed to parse auto-fix flag: %w", err)
		}
	}

	f.shared.Logger.DebugWithContext("parsed global flags", map[string]interface{}{
		"dry_run":   flags.DryRun,
		"force":     flags.Force,
		"verbose":   flags.Verbose,
		"timeout":   flags.Timeout.String(),
		"output":    flags.Output,
		"log_level": flags.LogLevel,
		"no_color":  flags.NoColor,
		"auto_fix":  flags.AutoFix,
	})

	return flags, nil
}

// ValidateGlobalFlags validates global flags for consistency
func (f *FlagParser) ValidateGlobalFlags(flags *GlobalFlags) error {
	// Validate timeout
	if flags.Timeout < 0 {
		return fmt.Errorf("timeout cannot be negative: %s", flags.Timeout)
	}
	if flags.Timeout > 24*time.Hour {
		return fmt.Errorf("timeout cannot exceed 24 hours: %s", flags.Timeout)
	}

	// Validate output format
	validOutputs := map[string]bool{
		"text": true,
		"json": true,
		"yaml": true,
	}
	if !validOutputs[flags.Output] {
		return fmt.Errorf("invalid output format '%s'. Valid formats: text, json, yaml", flags.Output)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[flags.LogLevel] {
		return fmt.Errorf("invalid log level '%s'. Valid levels: debug, info, warn, error", flags.LogLevel)
	}

	// Warn about conflicting flags
	if flags.Force && flags.DryRun {
		f.shared.Logger.WarnWithContext("force and dry-run flags are both set", map[string]interface{}{
			"force":   flags.Force,
			"dry_run": flags.DryRun,
		})
	}

	return nil
}

// ParseStringSliceFlag safely parses a string slice flag
func (f *FlagParser) ParseStringSliceFlag(cmd *cobra.Command, flagName string) ([]string, error) {
	value, err := cmd.Flags().GetStringSlice(flagName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s flag: %w", flagName, err)
	}

	// Filter out empty strings
	result := make([]string, 0, len(value))
	for _, v := range value {
		if v != "" {
			result = append(result, v)
		}
	}

	f.shared.Logger.DebugWithContext("parsed string slice flag", map[string]interface{}{
		"flag":   flagName,
		"values": result,
		"count":  len(result),
	})

	return result, nil
}

// ParseOptionalBoolFlag parses a boolean flag that may not exist
func (f *FlagParser) ParseOptionalBoolFlag(cmd *cobra.Command, flagName string, defaultValue bool) bool {
	if cmd.Flags().Lookup(flagName) == nil {
		return defaultValue
	}

	value, err := cmd.Flags().GetBool(flagName)
	if err != nil {
		f.shared.Logger.WarnWithContext("failed to parse optional bool flag", map[string]interface{}{
			"flag":          flagName,
			"error":         err.Error(),
			"default_used":  defaultValue,
		})
		return defaultValue
	}

	return value
}

// ParseOptionalStringFlag parses a string flag that may not exist
func (f *FlagParser) ParseOptionalStringFlag(cmd *cobra.Command, flagName string, defaultValue string) string {
	if cmd.Flags().Lookup(flagName) == nil {
		return defaultValue
	}

	value, err := cmd.Flags().GetString(flagName)
	if err != nil {
		f.shared.Logger.WarnWithContext("failed to parse optional string flag", map[string]interface{}{
			"flag":          flagName,
			"error":         err.Error(),
			"default_used":  defaultValue,
		})
		return defaultValue
	}

	return value
}

// ParseOptionalDurationFlag parses a duration flag that may not exist
func (f *FlagParser) ParseOptionalDurationFlag(cmd *cobra.Command, flagName string, defaultValue time.Duration) time.Duration {
	if cmd.Flags().Lookup(flagName) == nil {
		return defaultValue
	}

	value, err := cmd.Flags().GetDuration(flagName)
	if err != nil {
		f.shared.Logger.WarnWithContext("failed to parse optional duration flag", map[string]interface{}{
			"flag":          flagName,
			"error":         err.Error(),
			"default_used":  defaultValue.String(),
		})
		return defaultValue
	}

	return value
}

// GlobalFlags represents common global flags across all commands
type GlobalFlags struct {
	DryRun   bool
	Force    bool
	Verbose  bool
	Timeout  time.Duration
	Output   string
	LogLevel string
	NoColor  bool
	AutoFix  bool
}

// IsDestructive checks if the flags indicate a destructive operation
func (g *GlobalFlags) IsDestructive() bool {
	return g.Force && !g.DryRun
}

// ShouldConfirm checks if the operation should prompt for confirmation
func (g *GlobalFlags) ShouldConfirm() bool {
	return !g.Force && !g.DryRun
}

// GetEffectiveTimeout returns the effective timeout, considering environment and operation type
func (g *GlobalFlags) GetEffectiveTimeout(defaultTimeout time.Duration) time.Duration {
	if g.Timeout > 0 {
		return g.Timeout
	}
	return defaultTimeout
}