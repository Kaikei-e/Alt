package config_port

import (
	"testing"
	"time"
)

// TestConfigPortInterface verifies the interface is properly defined
func TestConfigPortInterface(t *testing.T) {
	// This test ensures the ConfigPort interface compiles correctly
	// Actual implementation testing will be done in the gateway layer
	var _ ConfigPort = (*mockConfigPort)(nil)
}

// mockConfigPort is a simple mock to verify interface compliance
type mockConfigPort struct{}

func (m *mockConfigPort) GetServerPort() int {
	return 9000
}

func (m *mockConfigPort) GetServerTimeouts() ServerTimeouts {
	return ServerTimeouts{
		Read:  30 * time.Second,
		Write: 30 * time.Second,
		Idle:  120 * time.Second,
	}
}

func (m *mockConfigPort) GetRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		ExternalAPIInterval: 5 * time.Second,
		FeedFetchLimit:      100,
		EnablePerHostLimit:  true,
	}
}

func (m *mockConfigPort) GetDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		MaxConnections:    25,
		ConnectionTimeout: 30 * time.Second,
		MaxIdleTime:       300 * time.Second,
	}
}

func (m *mockConfigPort) GetCacheConfig() CacheConfig {
	return CacheConfig{
		FeedCacheExpiry:   300 * time.Second,
		SearchCacheExpiry: 900 * time.Second,
		EnableCaching:     true,
	}
}

func (m *mockConfigPort) GetLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:  "info",
		Format: "json",
	}
}

func (m *mockConfigPort) Validate() error {
	return nil
}
