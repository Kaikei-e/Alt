package config_gateway

import (
	"alt/config"
	"alt/port/config_port"
	"testing"
	"time"
)

func TestConfigGateway_GetServerPort(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
	}
	
	gateway := NewConfigGateway(cfg)
	port := gateway.GetServerPort()
	
	if port != 8080 {
		t.Errorf("GetServerPort() = %v, want %v", port, 8080)
	}
}

func TestConfigGateway_GetServerTimeouts(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
	
	gateway := NewConfigGateway(cfg)
	timeouts := gateway.GetServerTimeouts()
	
	expected := config_port.ServerTimeouts{
		Read:  30 * time.Second,
		Write: 30 * time.Second,
		Idle:  120 * time.Second,
	}
	
	if timeouts != expected {
		t.Errorf("GetServerTimeouts() = %v, want %v", timeouts, expected)
	}
}

func TestConfigGateway_GetRateLimitConfig(t *testing.T) {
	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{
			ExternalAPIInterval: 5 * time.Second,
			FeedFetchLimit:      100,
		},
	}
	
	gateway := NewConfigGateway(cfg)
	rateLimitConfig := gateway.GetRateLimitConfig()
	
	if rateLimitConfig.ExternalAPIInterval != 5*time.Second {
		t.Errorf("GetRateLimitConfig().ExternalAPIInterval = %v, want %v", 
			rateLimitConfig.ExternalAPIInterval, 5*time.Second)
	}
	
	if rateLimitConfig.FeedFetchLimit != 100 {
		t.Errorf("GetRateLimitConfig().FeedFetchLimit = %v, want %v", 
			rateLimitConfig.FeedFetchLimit, 100)
	}
	
	if !rateLimitConfig.EnablePerHostLimit {
		t.Errorf("GetRateLimitConfig().EnablePerHostLimit = %v, want %v", 
			rateLimitConfig.EnablePerHostLimit, true)
	}
}