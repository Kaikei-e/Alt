// TDD Phase 3 - REFACTOR: Monitoring Integration Test
package test

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/service"
	"pre-processor-sidecar/utils"
)

// MockMonitorInoreaderClient for monitoring integration testing
type MockMonitorInoreaderClient struct {
	shouldFail bool
	callCount  int
}

func (m *MockMonitorInoreaderClient) FetchSubscriptionList(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	m.callCount++

	if m.shouldFail {
		return nil, fmt.Errorf("simulated API failure")
	}

	return map[string]interface{}{
		"subscriptions": []interface{}{
			map[string]interface{}{
				"id":    "feed/test1",
				"url":   "http://test1.com/rss",
				"title": "Test Feed 1",
			},
		},
	}, nil
}

func (m *MockMonitorInoreaderClient) FetchStreamContents(ctx context.Context, accessToken, streamID, continuationToken string, maxArticles int) (map[string]interface{}, error) {
	return m.FetchSubscriptionList(ctx, accessToken)
}

func (m *MockMonitorInoreaderClient) FetchUnreadStreamContents(ctx context.Context, accessToken, streamID, continuationToken string, maxArticles int) (map[string]interface{}, error) {
	return m.FetchSubscriptionList(ctx, accessToken)
}

func (m *MockMonitorInoreaderClient) RefreshToken(ctx context.Context, refreshToken string) (*models.InoreaderTokenResponse, error) {
	return &models.InoreaderTokenResponse{
		AccessToken:  "new_access_token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: "new_refresh_token",
	}, nil
}

func (m *MockMonitorInoreaderClient) ValidateToken(ctx context.Context, accessToken string) (bool, error) {
	return true, nil
}

func (m *MockMonitorInoreaderClient) MakeAuthenticatedRequestWithHeaders(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, map[string]string, error) {
	return map[string]interface{}{}, map[string]string{}, nil
}

func (m *MockMonitorInoreaderClient) ParseSubscriptionsResponse(response map[string]interface{}) ([]*models.Subscription, error) {
	subscriptions := []*models.Subscription{
		models.NewSubscription("feed/test1", "http://test1.com/rss", "Test Feed 1", "Test"),
	}
	return subscriptions, nil
}

func (m *MockMonitorInoreaderClient) ParseStreamContentsResponse(response map[string]interface{}) ([]*models.Article, string, error) {
	articles := []*models.Article{
		{
			ID:          models.NewUUID(),
			InoreaderID: "item/test1",
			Title:       "Test Article 1",
		},
	}
	return articles, "", nil
}

// TestInoreaderService_MonitoringIntegration tests monitoring integration
func TestInoreaderService_MonitoringIntegration(t *testing.T) {
	mockClient := &MockMonitorInoreaderClient{shouldFail: false}

	// Create service with monitoring enabled
	inoreaderService := service.NewInoreaderService(
		mockClient,
		nil, // No API usage repo for this test
		nil, // No token service for this test - will need to be mocked or handled
		slog.Default(),
	)
	defer inoreaderService.Close()

	// Test monitoring health check
	healthCheck := inoreaderService.GetMonitoringHealthCheck()
	if healthCheck["status"] != "healthy" {
		t.Errorf("Expected monitoring to be healthy, got %v", healthCheck["status"])
	}

	// Initially no metrics should exist
	initialMetrics := inoreaderService.GetMonitoringMetrics()
	if len(initialMetrics) != 0 {
		t.Errorf("Expected no initial metrics, got %d", len(initialMetrics))
	}

	// Note: This test demonstrates the monitoring integration structure
	// Full functionality testing requires proper token service mocking

	t.Logf("Monitoring integration structure validated successfully")
	t.Logf("Health check status: %v", healthCheck["status"])
	t.Logf("Metrics enabled: %v", healthCheck["metrics_enabled"])
	t.Logf("Initial metrics count: %d", len(initialMetrics))
}

// TestInoreaderService_MetricsCollection tests basic metrics collection
func TestInoreaderService_MetricsCollection(t *testing.T) {
	mockClient := &MockMonitorInoreaderClient{shouldFail: false}

	inoreaderService := service.NewInoreaderService(
		mockClient,
		nil,
		nil,
		slog.Default(),
	)
	defer inoreaderService.Close()

	// Test circuit breaker stats
	cbStats := inoreaderService.GetCircuitBreakerStats()
	if cbStats.State != utils.StateClosed {
		t.Errorf("Expected circuit breaker to start in CLOSED state, got %s", cbStats.State)
	}

	// Test monitoring health check fields
	healthCheck := inoreaderService.GetMonitoringHealthCheck()

	expectedFields := []string{
		"status", "metrics_enabled", "tracing_enabled",
		"metrics_count", "queue_length", "queue_capacity",
	}

	for _, field := range expectedFields {
		if _, exists := healthCheck[field]; !exists {
			t.Errorf("Expected health check field: %s", field)
		}
	}

	t.Logf("Circuit breaker initial state: %s", cbStats.State)
	t.Logf("Monitoring health check validated: %d fields present", len(healthCheck))
}

// TestInoreaderService_MonitoringCleanup tests proper resource cleanup
func TestInoreaderService_MonitoringCleanup(t *testing.T) {
	mockClient := &MockMonitorInoreaderClient{shouldFail: false}

	inoreaderService := service.NewInoreaderService(
		mockClient,
		nil,
		nil,
		slog.Default(),
	)

	// Get initial health status
	healthCheck := inoreaderService.GetMonitoringHealthCheck()
	if healthCheck["status"] != "healthy" {
		t.Errorf("Expected healthy status, got %v", healthCheck["status"])
	}

	// Test proper cleanup
	inoreaderService.Close()

	// Note: After Close(), the monitoring system should gracefully shutdown
	// In a full implementation, we might check that background goroutines have stopped
	t.Logf("Monitoring cleanup completed successfully")
}
