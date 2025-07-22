// Phase R4: 統合設定管理 - 階層的設定管理（環境変数・ファイル・デフォルト値）
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ConfigSource defines where configuration values come from
type ConfigSource int

const (
	// DefaultSource indicates the value comes from default configuration
	DefaultSource ConfigSource = iota
	// FileSource indicates the value comes from a configuration file
	FileSource
	// EnvironmentSource indicates the value comes from environment variables
	EnvironmentSource
	// OverrideSource indicates the value was explicitly overridden
	OverrideSource
)

// ConfigValue holds a configuration value with its source and metadata
type ConfigValue struct {
	Value       interface{}   `json:"value"`
	Source      ConfigSource  `json:"source"`
	Key         string        `json:"key"`
	Type        string        `json:"type"`
	Description string        `json:"description,omitempty"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// ConfigManager manages hierarchical configuration with multiple sources
type ConfigManager struct {
	values        map[string]*ConfigValue
	defaults      map[string]interface{}
	envPrefix     string
	configPaths   []string
	watchers      []ConfigWatcher
	mutex         sync.RWMutex
	validators    map[string]ConfigValidator
}

// ConfigWatcher is notified when configuration changes
type ConfigWatcher interface {
	OnConfigChanged(key string, oldValue, newValue interface{}) error
}

// ConfigValidator validates configuration values
type ConfigValidator interface {
	Validate(key string, value interface{}) error
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(envPrefix string) *ConfigManager {
	return &ConfigManager{
		values:     make(map[string]*ConfigValue),
		defaults:   make(map[string]interface{}),
		envPrefix:  envPrefix,
		validators: make(map[string]ConfigValidator),
	}
}

// SetDefaults sets default configuration values
func (cm *ConfigManager) SetDefaults(defaults map[string]interface{}) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for key, value := range defaults {
		cm.defaults[key] = value
		if _, exists := cm.values[key]; !exists {
			cm.values[key] = &ConfigValue{
				Value:     value,
				Source:    DefaultSource,
				Key:       key,
				Type:      reflect.TypeOf(value).String(),
				UpdatedAt: time.Now(),
			}
		}
	}
}

// AddConfigPath adds a path to search for configuration files
func (cm *ConfigManager) AddConfigPath(path string) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	
	cm.configPaths = append(cm.configPaths, path)
}

// LoadConfig loads configuration from all sources
func (cm *ConfigManager) LoadConfig() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// 1. Load from configuration files
	if err := cm.loadFromFiles(); err != nil {
		return fmt.Errorf("failed to load from config files: %w", err)
	}

	// 2. Load from environment variables
	if err := cm.loadFromEnvironment(); err != nil {
		return fmt.Errorf("failed to load from environment: %w", err)
	}

	// 3. Validate all configurations
	if err := cm.validateAll(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	return nil
}

// loadFromFiles loads configuration from files
func (cm *ConfigManager) loadFromFiles() error {
	for _, path := range cm.configPaths {
		if err := cm.loadFromFile(path); err != nil {
			// Log warning but continue with other files
			continue
		}
	}
	return nil
}

// loadFromFile loads configuration from a specific file
func (cm *ConfigManager) loadFromFile(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // File doesn't exist, skip
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var config map[string]interface{}
	
	// Support both JSON and environment-style config files
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse JSON config file %s: %w", path, err)
		}
	case ".env":
		config = cm.parseEnvFile(string(data))
	default:
		// Try JSON first, then env format
		if err := json.Unmarshal(data, &config); err != nil {
			config = cm.parseEnvFile(string(data))
		}
	}

	// Apply configuration values
	for key, value := range config {
		cm.setValue(key, value, FileSource, path)
	}

	return nil
}

// parseEnvFile parses environment-style configuration
func (cm *ConfigManager) parseEnvFile(content string) map[string]interface{} {
	config := make(map[string]interface{})
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
			(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
			value = value[1 : len(value)-1]
		}

		config[key] = cm.parseValue(value)
	}

	return config
}

// loadFromEnvironment loads configuration from environment variables
func (cm *ConfigManager) loadFromEnvironment() error {
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Check if this environment variable matches our prefix
		if cm.envPrefix != "" && strings.HasPrefix(key, cm.envPrefix+"_") {
			configKey := strings.TrimPrefix(key, cm.envPrefix+"_")
			configKey = strings.ToLower(strings.ReplaceAll(configKey, "_", "."))
			cm.setValue(configKey, cm.parseValue(value), EnvironmentSource, "ENV:"+key)
		}
	}

	return nil
}

// parseValue attempts to parse a string value into appropriate type
func (cm *ConfigManager) parseValue(value string) interface{} {
	// Try boolean
	if value == "true" || value == "false" {
		return value == "true"
	}

	// Try integer
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal
	}

	// Try float
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal
	}

	// Try duration
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}

	// Return as string
	return value
}

// setValue sets a configuration value
func (cm *ConfigManager) setValue(key string, value interface{}, source ConfigSource, sourceInfo string) {
	oldValue := cm.values[key]
	
	cm.values[key] = &ConfigValue{
		Value:       value,
		Source:      source,
		Key:         key,
		Type:        reflect.TypeOf(value).String(),
		Description: fmt.Sprintf("Loaded from %s", sourceInfo),
		UpdatedAt:   time.Now(),
	}

	// Notify watchers
	for _, watcher := range cm.watchers {
		var old interface{}
		if oldValue != nil {
			old = oldValue.Value
		}
		watcher.OnConfigChanged(key, old, value)
	}
}

// Get retrieves a configuration value
func (cm *ConfigManager) Get(key string) (interface{}, bool) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	if value, exists := cm.values[key]; exists {
		return value.Value, true
	}
	return nil, false
}

// GetString retrieves a string configuration value
func (cm *ConfigManager) GetString(key string) string {
	if value, exists := cm.Get(key); exists {
		if str, ok := value.(string); ok {
			return str
		}
		return fmt.Sprintf("%v", value)
	}
	return ""
}

// GetInt retrieves an integer configuration value
func (cm *ConfigManager) GetInt(key string) int {
	if value, exists := cm.Get(key); exists {
		switch v := value.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		case string:
			if intVal, err := strconv.Atoi(v); err == nil {
				return intVal
			}
		}
	}
	return 0
}

// GetBool retrieves a boolean configuration value
func (cm *ConfigManager) GetBool(key string) bool {
	if value, exists := cm.Get(key); exists {
		if boolVal, ok := value.(bool); ok {
			return boolVal
		}
		if strVal, ok := value.(string); ok {
			return strVal == "true"
		}
	}
	return false
}

// GetDuration retrieves a duration configuration value
func (cm *ConfigManager) GetDuration(key string) time.Duration {
	if value, exists := cm.Get(key); exists {
		if duration, ok := value.(time.Duration); ok {
			return duration
		}
		if strVal, ok := value.(string); ok {
			if duration, err := time.ParseDuration(strVal); err == nil {
				return duration
			}
		}
	}
	return 0
}

// Set explicitly sets a configuration value (override source)
func (cm *ConfigManager) Set(key string, value interface{}) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Validate if validator exists
	if validator, exists := cm.validators[key]; exists {
		if err := validator.Validate(key, value); err != nil {
			return fmt.Errorf("validation failed for key %s: %w", key, err)
		}
	}

	cm.setValue(key, value, OverrideSource, "Explicit override")
	return nil
}

// AddWatcher adds a configuration watcher
func (cm *ConfigManager) AddWatcher(watcher ConfigWatcher) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	
	cm.watchers = append(cm.watchers, watcher)
}

// AddValidator adds a configuration validator for a specific key
func (cm *ConfigManager) AddValidator(key string, validator ConfigValidator) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	
	cm.validators[key] = validator
}

// validateAll validates all configuration values
func (cm *ConfigManager) validateAll() error {
	for key, validator := range cm.validators {
		if value, exists := cm.values[key]; exists {
			if err := validator.Validate(key, value.Value); err != nil {
				return fmt.Errorf("validation failed for key %s: %w", key, err)
			}
		}
	}
	return nil
}

// GetAllValues returns all configuration values
func (cm *ConfigManager) GetAllValues() map[string]*ConfigValue {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	result := make(map[string]*ConfigValue)
	for key, value := range cm.values {
		// Create a copy to avoid concurrent modification
		result[key] = &ConfigValue{
			Value:       value.Value,
			Source:      value.Source,
			Key:         value.Key,
			Type:        value.Type,
			Description: value.Description,
			UpdatedAt:   value.UpdatedAt,
		}
	}
	return result
}

// GetConfigInfo returns information about current configuration
func (cm *ConfigManager) GetConfigInfo() *ConfigInfo {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	info := &ConfigInfo{
		TotalValues:     len(cm.values),
		Sources:         make(map[string]int),
		ConfigPaths:     cm.configPaths,
		EnvironmentPrefix: cm.envPrefix,
	}

	for _, value := range cm.values {
		source := ""
		switch value.Source {
		case DefaultSource:
			source = "default"
		case FileSource:
			source = "file"
		case EnvironmentSource:
			source = "environment"
		case OverrideSource:
			source = "override"
		}
		info.Sources[source]++
	}

	return info
}

// ConfigInfo provides information about configuration state
type ConfigInfo struct {
	TotalValues       int            `json:"total_values"`
	Sources           map[string]int `json:"sources"`
	ConfigPaths       []string       `json:"config_paths"`
	EnvironmentPrefix string         `json:"environment_prefix"`
}

// Reload reloads configuration from all sources
func (cm *ConfigManager) Reload() error {
	return cm.LoadConfig()
}