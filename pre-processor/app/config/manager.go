package config

import (
	"fmt"
	"sync"

	"log/slog"
)

type ConfigManager struct {
	config *Config
	mu     sync.RWMutex
	logger *slog.Logger
}

func NewConfigManager(config *Config, logger *slog.Logger) *ConfigManager {
	return &ConfigManager{
		config: config,
		logger: logger,
	}
}

func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	configCopy := *cm.config
	return &configCopy
}

func (cm *ConfigManager) UpdateConfig(newConfig *Config) error {
	if err := validateConfig(newConfig); err != nil {
		return fmt.Errorf("new config validation failed: %w", err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	oldConfig := cm.config
	cm.config = newConfig

	if cm.logger != nil {
		cm.logger.Info("configuration updated",
			"old_http_timeout", oldConfig.HTTP.Timeout,
			"new_http_timeout", newConfig.HTTP.Timeout,
			"old_retry_attempts", oldConfig.Retry.MaxAttempts,
			"new_retry_attempts", newConfig.Retry.MaxAttempts)
	}

	return nil
}
