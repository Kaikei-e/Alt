// ABOUTME: This file contains comprehensive TDD tests for user subscription synchronization
// ABOUTME: Tests authentication, tenant isolation, and error handling scenarios

package service_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"alt/shared/auth-lib-go/pkg/auth"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"pre-processor/service"
	"pre-processor/test/mocks"
)

func TestUserSyncService_SyncUserSubscriptions(t *testing.T) {
	// Test data setup
	testUserID := uuid.New()
	testTenantID := uuid.New()
	testSubscriptions := []service.Subscription{
		{
			ID:          "sub1",
			Title:       "Tech News",
			URL:         "http://example.com/tech-news.rss",
			Description: "Latest technology news",
		},
		{
			ID:          "sub2",
			Title:       "Science Daily",
			URL:         "http://example.com/science.rss",
			Description: "Daily science updates",
		},
	}

	tests := map[string]struct {
		setupMocks    func(*mocks.MockInoreaderClient, *mocks.MockUserSubscriptionRepository)
		setupContext  func() context.Context
		expectedError string
		description   string
	}{
		"success_case_with_authenticated_user": {
			description: "Should successfully sync subscriptions for authenticated user",
			setupContext: func() context.Context {
				userCtx := &auth.UserContext{
					UserID:   testUserID,
					TenantID: testTenantID,
					Email:    "test@example.com",
				}
				return context.WithValue(context.Background(), "user", userCtx)
			},
			setupMocks: func(mockClient *mocks.MockInoreaderClient, mockRepo *mocks.MockUserSubscriptionRepository) {
				mockClient.EXPECT().
					GetUserSubscriptions(gomock.Any(), testUserID.String()).
					Return(testSubscriptions, nil).
					Times(1)

				mockRepo.EXPECT().
					SaveUserSubscriptions(
						gomock.Any(),
						testTenantID.String(),
						testUserID.String(),
						testSubscriptions,
					).
					Return(nil).
					Times(1)
			},
			expectedError: "",
		},
		"error_case_missing_user_context": {
			description: "Should fail when user context is missing",
			setupContext: func() context.Context {
				return context.Background() // No user context
			},
			setupMocks: func(mockClient *mocks.MockInoreaderClient, mockRepo *mocks.MockUserSubscriptionRepository) {
				// No mock expectations - should fail before reaching clients
			},
			expectedError: "authentication required: user context not found",
		},
		"error_case_nil_user_context": {
			description: "Should fail when user context is nil",
			setupContext: func() context.Context {
				return context.WithValue(context.Background(), "user", nil)
			},
			setupMocks: func(mockClient *mocks.MockInoreaderClient, mockRepo *mocks.MockUserSubscriptionRepository) {
				// No mock expectations - should fail before reaching clients
			},
			expectedError: "authentication required: user context not found",
		},
		"error_case_invalid_user_context_type": {
			description: "Should fail when user context has wrong type",
			setupContext: func() context.Context {
				return context.WithValue(context.Background(), "user", "invalid-type")
			},
			setupMocks: func(mockClient *mocks.MockInoreaderClient, mockRepo *mocks.MockUserSubscriptionRepository) {
				// No mock expectations - should fail before reaching clients
			},
			expectedError: "authentication required: user context not found",
		},
		"error_case_inoreader_client_failure": {
			description: "Should fail when Inoreader client returns error",
			setupContext: func() context.Context {
				userCtx := &auth.UserContext{
					UserID:   testUserID,
					TenantID: testTenantID,
					Email:    "test@example.com",
				}
				return context.WithValue(context.Background(), "user", userCtx)
			},
			setupMocks: func(mockClient *mocks.MockInoreaderClient, mockRepo *mocks.MockUserSubscriptionRepository) {
				mockClient.EXPECT().
					GetUserSubscriptions(gomock.Any(), testUserID.String()).
					Return(nil, errors.New("API rate limit exceeded")).
					Times(1)

				// Repository should not be called
			},
			expectedError: "failed to get user subscriptions: API rate limit exceeded",
		},
		"error_case_repository_save_failure": {
			description: "Should fail when repository save returns error",
			setupContext: func() context.Context {
				userCtx := &auth.UserContext{
					UserID:   testUserID,
					TenantID: testTenantID,
					Email:    "test@example.com",
				}
				return context.WithValue(context.Background(), "user", userCtx)
			},
			setupMocks: func(mockClient *mocks.MockInoreaderClient, mockRepo *mocks.MockUserSubscriptionRepository) {
				mockClient.EXPECT().
					GetUserSubscriptions(gomock.Any(), testUserID.String()).
					Return(testSubscriptions, nil).
					Times(1)

				mockRepo.EXPECT().
					SaveUserSubscriptions(
						gomock.Any(),
						testTenantID.String(),
						testUserID.String(),
						testSubscriptions,
					).
					Return(errors.New("database connection failed")).
					Times(1)
			},
			expectedError: "failed to save user subscriptions: database connection failed",
		},
		"success_case_empty_subscriptions": {
			description: "Should handle empty subscription list successfully",
			setupContext: func() context.Context {
				userCtx := &auth.UserContext{
					UserID:   testUserID,
					TenantID: testTenantID,
					Email:    "test@example.com",
				}
				return context.WithValue(context.Background(), "user", userCtx)
			},
			setupMocks: func(mockClient *mocks.MockInoreaderClient, mockRepo *mocks.MockUserSubscriptionRepository) {
				emptySubscriptions := []service.Subscription{}

				mockClient.EXPECT().
					GetUserSubscriptions(gomock.Any(), testUserID.String()).
					Return(emptySubscriptions, nil).
					Times(1)

				mockRepo.EXPECT().
					SaveUserSubscriptions(
						gomock.Any(),
						testTenantID.String(),
						testUserID.String(),
						emptySubscriptions,
					).
					Return(nil).
					Times(1)
			},
			expectedError: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockInoreaderClient(ctrl)
			mockRepo := mocks.NewMockUserSubscriptionRepository(ctrl)
			logger := slog.Default()

			// Setup test expectations
			tc.setupMocks(mockClient, mockRepo)

			// Create service
			serviceInstance := service.NewUserSyncService(mockClient, mockRepo, logger)

			// Setup context
			ctx := tc.setupContext()

			// Execute test
			err := serviceInstance.SyncUserSubscriptions(ctx)

			// Verify results
			if tc.expectedError != "" {
				require.Error(t, err, tc.description)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err, tc.description)
			}
		})
	}
}

func TestUserSyncService_GetUserSubscriptions(t *testing.T) {
	// Test data setup
	testUserID := uuid.New()
	testTenantID := uuid.New()
	testSubscriptions := []service.Subscription{
		{
			ID:          "sub1",
			Title:       "Tech News",
			URL:         "http://example.com/tech-news.rss",
			Description: "Latest technology news",
		},
	}

	tests := map[string]struct {
		setupMocks     func(*mocks.MockInoreaderClient, *mocks.MockUserSubscriptionRepository)
		setupContext   func() context.Context
		expectedResult []service.Subscription
		expectedError  string
		description    string
	}{
		"success_case_get_subscriptions": {
			description: "Should successfully retrieve user subscriptions",
			setupContext: func() context.Context {
				userCtx := &auth.UserContext{
					UserID:   testUserID,
					TenantID: testTenantID,
					Email:    "test@example.com",
				}
				return context.WithValue(context.Background(), "user", userCtx)
			},
			setupMocks: func(mockClient *mocks.MockInoreaderClient, mockRepo *mocks.MockUserSubscriptionRepository) {
				mockClient.EXPECT().
					GetUserSubscriptions(gomock.Any(), testUserID.String()).
					Return(testSubscriptions, nil).
					Times(1)
			},
			expectedResult: testSubscriptions,
			expectedError:  "",
		},
		"error_case_missing_user_context": {
			description: "Should fail when user context is missing",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(mockClient *mocks.MockInoreaderClient, mockRepo *mocks.MockUserSubscriptionRepository) {
				// No expectations
			},
			expectedResult: nil,
			expectedError:  "authentication required: user context not found",
		},
		"error_case_client_failure": {
			description: "Should fail when client returns error",
			setupContext: func() context.Context {
				userCtx := &auth.UserContext{
					UserID:   testUserID,
					TenantID: testTenantID,
					Email:    "test@example.com",
				}
				return context.WithValue(context.Background(), "user", userCtx)
			},
			setupMocks: func(mockClient *mocks.MockInoreaderClient, mockRepo *mocks.MockUserSubscriptionRepository) {
				mockClient.EXPECT().
					GetUserSubscriptions(gomock.Any(), testUserID.String()).
					Return(nil, errors.New("network timeout")).
					Times(1)
			},
			expectedResult: nil,
			expectedError:  "failed to get user subscriptions: network timeout",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockInoreaderClient(ctrl)
			mockRepo := mocks.NewMockUserSubscriptionRepository(ctrl)
			logger := slog.Default()

			// Setup test expectations
			tc.setupMocks(mockClient, mockRepo)

			// Create service
			serviceInstance := service.NewUserSyncService(mockClient, mockRepo, logger)

			// Setup context
			ctx := tc.setupContext()

			// Execute test
			result, err := serviceInstance.GetUserSubscriptions(ctx)

			// Verify results
			if tc.expectedError != "" {
				require.Error(t, err, tc.description)
				assert.Contains(t, err.Error(), tc.expectedError)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err, tc.description)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

// TestUserSyncService_TenantIsolation verifies that tenant isolation is properly maintained
func TestUserSyncService_TenantIsolation(t *testing.T) {
	// Test multiple tenants to ensure proper isolation
	tenant1ID := uuid.New()
	tenant2ID := uuid.New()
	user1ID := uuid.New()
	user2ID := uuid.New()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockInoreaderClient(ctrl)
	mockRepo := mocks.NewMockUserSubscriptionRepository(ctrl)
	logger := slog.Default()

	serviceInstance := service.NewUserSyncService(mockClient, mockRepo, logger)

	// Setup expectations for tenant 1
	subscriptions1 := []service.Subscription{{ID: "sub1", Title: "Tenant 1 Feed"}}
	mockClient.EXPECT().
		GetUserSubscriptions(gomock.Any(), user1ID.String()).
		Return(subscriptions1, nil).
		Times(1)

	mockRepo.EXPECT().
		SaveUserSubscriptions(
			gomock.Any(),
			tenant1ID.String(), // Ensure tenant 1 ID is used
			user1ID.String(),
			subscriptions1,
		).
		Return(nil).
		Times(1)

	// Setup expectations for tenant 2
	subscriptions2 := []service.Subscription{{ID: "sub2", Title: "Tenant 2 Feed"}}
	mockClient.EXPECT().
		GetUserSubscriptions(gomock.Any(), user2ID.String()).
		Return(subscriptions2, nil).
		Times(1)

	mockRepo.EXPECT().
		SaveUserSubscriptions(
			gomock.Any(),
			tenant2ID.String(), // Ensure tenant 2 ID is used
			user2ID.String(),
			subscriptions2,
		).
		Return(nil).
		Times(1)

	// Test tenant 1
	ctx1 := context.WithValue(context.Background(), "user", &auth.UserContext{
		UserID:   user1ID,
		TenantID: tenant1ID,
		Email:    "user1@tenant1.com",
	})

	err1 := serviceInstance.SyncUserSubscriptions(ctx1)
	require.NoError(t, err1)

	// Test tenant 2
	ctx2 := context.WithValue(context.Background(), "user", &auth.UserContext{
		UserID:   user2ID,
		TenantID: tenant2ID,
		Email:    "user2@tenant2.com",
	})

	err2 := serviceInstance.SyncUserSubscriptions(ctx2)
	require.NoError(t, err2)
}

// TestUserSyncService_Constructor verifies proper service construction
func TestUserSyncService_Constructor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := mocks.NewMockInoreaderClient(ctrl)
	mockRepo := mocks.NewMockUserSubscriptionRepository(ctrl)
	logger := slog.Default()

	serviceInstance := service.NewUserSyncService(mockClient, mockRepo, logger)

	// Verify service is properly constructed
	assert.NotNil(t, serviceInstance)

	// Verify interface compliance
	var _ service.UserSyncService = serviceInstance
}
