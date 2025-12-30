// Package config provides Viper-based configuration management for altctl
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config represents the complete altctl configuration
type Config struct {
	Project  ProjectConfig  `mapstructure:"project"`
	Compose  ComposeConfig  `mapstructure:"compose"`
	Defaults DefaultsConfig `mapstructure:"defaults"`
	Stacks   StacksConfig   `mapstructure:"stacks"`
	Logging  LoggingConfig  `mapstructure:"logging"`
	Output   OutputConfig   `mapstructure:"output"`
}

// ProjectConfig contains project-level settings
type ProjectConfig struct {
	Root          string `mapstructure:"root"`
	DockerContext string `mapstructure:"docker_context"`
}

// ComposeConfig contains Docker Compose file settings
type ComposeConfig struct {
	Dir      string `mapstructure:"dir"`
	BaseFile string `mapstructure:"base_file"`
}

// DefaultsConfig contains default behavior settings
type DefaultsConfig struct {
	Stacks []string `mapstructure:"stacks"`
}

// StacksConfig is a map of stack-specific overrides
type StacksConfig map[string]StackOverride

// StackOverride contains per-stack configuration overrides
type StackOverride struct {
	RequiresGPU    bool          `mapstructure:"requires_gpu"`
	StartupTimeout time.Duration `mapstructure:"startup_timeout"`
	ExtraFiles     []string      `mapstructure:"extra_files"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// OutputConfig contains output formatting settings
type OutputConfig struct {
	Colors   bool `mapstructure:"colors"`
	Progress bool `mapstructure:"progress"`
}

// Load reads configuration from file and environment variables
func Load(cfgFile, projectDir string) (*Config, error) {
	v := viper.New()

	// Set config file if specified
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		// Search paths for .altctl.yaml
		v.SetConfigName(".altctl")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.config/altctl")

		// Also search in project directory if specified
		if projectDir != "" {
			v.AddConfigPath(projectDir)
		}
	}

	// Environment variables
	v.SetEnvPrefix("ALTCTL")
	v.AutomaticEnv()

	// Set defaults
	setDefaults(v)

	// Override project root if specified via flag
	if projectDir != "" {
		v.Set("project.root", projectDir)
	}

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		// Config file not found is OK, use defaults
	}

	// Auto-detect project root if not set
	if v.GetString("project.root") == "" {
		root := detectProjectRoot()
		v.Set("project.root", root)
	}

	// Unmarshal into struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Validate configuration
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

// setDefaults configures default values
func setDefaults(v *viper.Viper) {
	// Project defaults
	v.SetDefault("project.docker_context", "default")

	// Compose defaults
	v.SetDefault("compose.dir", "compose")
	v.SetDefault("compose.base_file", "base.yaml")

	// Default stacks to start
	v.SetDefault("defaults.stacks", []string{"db", "auth", "core", "workers"})

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")

	// Output defaults
	v.SetDefault("output.colors", true)
	v.SetDefault("output.progress", true)
}

// detectProjectRoot attempts to find the Alt project root directory
func detectProjectRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}

	// Walk up the directory tree looking for project markers
	dir := cwd
	for {
		// Check for compose.yaml (current structure)
		if _, err := os.Stat(filepath.Join(dir, "compose.yaml")); err == nil {
			return dir
		}
		// Check for compose/ directory (new structure)
		if _, err := os.Stat(filepath.Join(dir, "compose")); err == nil {
			return dir
		}
		// Check for .altctl.yaml
		if _, err := os.Stat(filepath.Join(dir, ".altctl.yaml")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return cwd
		}
		dir = parent
	}
}

// validate checks the configuration for errors
func validate(cfg *Config) error {
	// Validate project root exists
	if cfg.Project.Root != "" {
		if _, err := os.Stat(cfg.Project.Root); os.IsNotExist(err) {
			return fmt.Errorf("project root does not exist: %s", cfg.Project.Root)
		}
	}

	// Validate logging level
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[cfg.Logging.Level] {
		return fmt.Errorf("invalid logging level: %s (must be debug, info, warn, or error)", cfg.Logging.Level)
	}

	// Validate logging format
	validFormats := map[string]bool{"text": true, "json": true}
	if !validFormats[cfg.Logging.Format] {
		return fmt.Errorf("invalid logging format: %s (must be text or json)", cfg.Logging.Format)
	}

	return nil
}

// GetComposeFilePath returns the full path to a compose file
func (c *Config) GetComposeFilePath(filename string) string {
	return filepath.Join(c.Project.Root, c.Compose.Dir, filename)
}

// GetBaseComposeFile returns the path to the base compose file
func (c *Config) GetBaseComposeFile() string {
	return c.GetComposeFilePath(c.Compose.BaseFile)
}
