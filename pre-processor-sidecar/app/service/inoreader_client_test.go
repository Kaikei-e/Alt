package service

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"pre-processor-sidecar/mocks"
)

// TEST: リトライロジックテスト - 適切なモック実装でHTTPコールを防止
func TestInoreaderClient_RetryOnTransientFailures(t *testing.T) {
	tests := []struct {
		name          string
		mockResponses []func() (map[string]interface{}, error)
		shouldSucceed bool
	}{
		{
			name: "403 forbidden - リトライして成功",
			mockResponses: []func() (map[string]interface{}, error){
				func() (map[string]interface{}, error) {
					return nil, fmt.Errorf("API request failed with status 403")
				},
				func() (map[string]interface{}, error) {
					return map[string]interface{}{"subscriptions": []interface{}{}}, nil
				},
			},
			shouldSucceed: true,
		},
		{
			name: "非リトライ対象エラー - 即座に失敗",
			mockResponses: []func() (map[string]interface{}, error){
				func() (map[string]interface{}, error) {
					return nil, fmt.Errorf("API request failed with status 400")
				},
			},
			shouldSucceed: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOAuth2 := mocks.NewMockOAuth2Driver(ctrl)
			
			// Track call count within the mock
			callCount := 0
			
			// Mock MakeAuthenticatedRequest with proper state tracking
			mockOAuth2.EXPECT().
				MakeAuthenticatedRequest(gomock.Any(), "test_token", "/subscription/list", gomock.Any()).
				DoAndReturn(func(ctx context.Context, token, endpoint string, params map[string]string) (map[string]interface{}, error) {
					if callCount < len(tt.mockResponses) {
						response, err := tt.mockResponses[callCount]()
						callCount++
						return response, err
					}
					// Should not reach here in normal test cases
					return nil, fmt.Errorf("unexpected call count: %d", callCount)
				}).
				AnyTimes() // Allow multiple calls for retry logic

			client := NewInoreaderClient(mockOAuth2, slog.Default())
			
			// FetchSubscriptionListWithRetryメソッドをテスト
			result, err := client.FetchSubscriptionListWithRetry(context.Background(), "test_token")
			
			if tt.shouldSucceed {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TEST: リトライ設定テスト - 基本的なコンフィグ検証
func TestInoreaderClient_RetryConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOAuth2 := mocks.NewMockOAuth2Driver(ctrl)
	client := NewInoreaderClient(mockOAuth2, slog.Default())
	
	// InoreaderClientのデフォルト設定を検証
	assert.NotNil(t, client)
	
	// Test default retry configuration
	retryConfig := client.GetRetryConfig()
	assert.NotNil(t, retryConfig)
	assert.Equal(t, 3, retryConfig.MaxRetries)
	assert.Equal(t, 5*time.Second, retryConfig.InitialDelay)
	assert.Equal(t, 30*time.Second, retryConfig.MaxDelay)
	assert.Equal(t, 2.0, retryConfig.Multiplier)
	
	// Test custom retry configuration
	customConfig := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 2 * time.Second,
		MaxDelay:     60 * time.Second,
		Multiplier:   1.5,
	}
	client.SetRetryConfig(customConfig)
	
	updatedConfig := client.GetRetryConfig()
	assert.Equal(t, customConfig.MaxRetries, updatedConfig.MaxRetries)
	assert.Equal(t, customConfig.InitialDelay, updatedConfig.InitialDelay)
	assert.Equal(t, customConfig.MaxDelay, updatedConfig.MaxDelay)
	assert.Equal(t, customConfig.Multiplier, updatedConfig.Multiplier)
}

// TEST: エラー分類テスト - isRetryableError関数の動作検証
func TestInoreaderClient_ErrorClassification(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		expectRetryable bool
	}{
		{
			name:            "403 Forbidden - リトライ可能",
			err:             fmt.Errorf("API request failed with status 403"),
			expectRetryable: true,
		},
		{
			name:            "タイムアウトエラー - リトライ可能", 
			err:             fmt.Errorf("request timeout occurred"),
			expectRetryable: true,
		},
		{
			name:            "接続拒否 - リトライ可能",
			err:             fmt.Errorf("connection refused"),
			expectRetryable: true,
		},
		{
			name:            "400 Bad Request - リトライ不可",
			err:             fmt.Errorf("API request failed with status 400"),
			expectRetryable: false,
		},
		{
			name:            "成功ケース",
			err:             nil,
			expectRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test isRetryableError function directly
			result := isRetryableError(tt.err)
			assert.Equal(t, tt.expectRetryable, result)
		})
	}
}