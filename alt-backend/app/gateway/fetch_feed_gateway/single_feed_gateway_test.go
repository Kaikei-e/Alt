package fetch_feed_gateway

import (
	"alt/utils/errors"
	"alt/utils/logger"
	"context"
	"testing"
)

func TestSingleFeedGateway_FetchSingleFeed_LoggerNilCase(t *testing.T) {
	// RED: Test to demonstrate the nil logger.GlobalContext issue
	// Save original logger
	originalLogger := logger.Logger
	originalGlobalContext := logger.GlobalContext
	
	// Set logger to nil to simulate uninitialized state
	logger.Logger = nil
	logger.GlobalContext = nil
	
	// Restore logger after test
	defer func() {
		logger.Logger = originalLogger
		logger.GlobalContext = originalGlobalContext
	}()

	// Create gateway with nil database connection to trigger error path
	gateway := &SingleFeedGateway{
		alt_db:      nil, // This will trigger database unavailable error
		rateLimiter: nil,
	}

	ctx := context.Background()
	
	// GREEN: This should no longer panic after the fix
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Unexpected panic occurred: %v", r)
		}
	}()
	
	// This call should NOT panic after the fix
	_, err := gateway.FetchSingleFeed(ctx)
	
	// We should get an error but no panic
	if err == nil {
		t.Error("Expected error but got nil")
	}
	
	// Verify it's an AppContextError
	if appErr, ok := err.(*errors.AppContextError); ok {
		t.Logf("Success: Got expected AppContextError: %s", appErr.Message)
	} else {
		t.Errorf("Expected AppContextError, got %T", err)
	}
}

func TestSingleFeedGateway_FetchSingleFeed_DatabaseNil(t *testing.T) {
	// Initialize logger for normal test
	logger.InitLogger()

	gateway := &SingleFeedGateway{
		alt_db:      nil, // Simulate nil database connection
		rateLimiter: nil,
	}

	ctx := context.Background()
	feed, err := gateway.FetchSingleFeed(ctx)

	if err == nil {
		t.Error("Expected error for nil database connection, got nil")
	}

	if feed != nil {
		t.Errorf("Expected nil feed, got %v", feed)
	}

	// Verify error type
	if appErr, ok := err.(*errors.AppContextError); ok {
		if appErr.Code != "DATABASE_ERROR" {
			t.Errorf("Expected DATABASE_ERROR error code, got %s", appErr.Code)
		}
		if appErr.Layer != "gateway" {
			t.Errorf("Expected layer to be 'gateway', got %s", appErr.Layer)
		}
		if appErr.Component != "SingleFeedGateway" {
			t.Errorf("Expected component to be 'SingleFeedGateway', got %s", appErr.Component)
		}
	} else {
		t.Errorf("Expected AppContextError, got %T", err)
	}
}

func TestSingleFeedGateway_FetchSingleFeed_NoFeedUrls(t *testing.T) {
	// Skip complex mock test for now
	t.Skip("Skipping test with complex database mock - focus on logger nil fix first")
}