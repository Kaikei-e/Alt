// ABOUTME: This file handles user-specific subscription synchronization
// ABOUTME: It processes user data with proper tenant isolation and authentication

package service

import (
	"context"
	"fmt"
	"log/slog"

	"pre-processor/internal/auth"
	"pre-processor/internal/auth/middleware"
)

type userSyncService struct {
	inoreaderClient InoreaderClient
	dbRepository    UserSubscriptionRepository
	logger          *slog.Logger
}

func NewUserSyncService(
	inoreaderClient InoreaderClient,
	dbRepository UserSubscriptionRepository,
	logger *slog.Logger,
) UserSyncService {
	return &userSyncService{
		inoreaderClient: inoreaderClient,
		dbRepository:    dbRepository,
		logger:          logger,
	}
}

func (s *userSyncService) SyncUserSubscriptions(ctx context.Context) error {
	// コンテキストからユーザー情報を取得
	user, ok := ctx.Value(middleware.UserContextKey).(*auth.UserContext)
	if !ok || user == nil {
		s.logger.Error("user context not found")
		return fmt.Errorf("authentication required: user context not found")
	}

	s.logger.Info("starting user subscription sync",
		"user_id", user.UserID,
		"tenant_id", user.TenantID,
		"email", user.Email)

	// ユーザー固有のサブスクリプション同期
	subscriptions, err := s.inoreaderClient.GetUserSubscriptions(ctx, user.UserID.String())
	if err != nil {
		s.logger.Error("failed to get user subscriptions",
			"error", err,
			"user_id", user.UserID)
		return fmt.Errorf("failed to get user subscriptions: %w", err)
	}

	s.logger.Info("retrieved user subscriptions",
		"user_id", user.UserID,
		"subscription_count", len(subscriptions))

	// テナント別に処理
	err = s.dbRepository.SaveUserSubscriptions(
		ctx,
		user.TenantID.String(),
		user.UserID.String(),
		subscriptions,
	)
	if err != nil {
		s.logger.Error("failed to save user subscriptions",
			"error", err,
			"user_id", user.UserID,
			"tenant_id", user.TenantID)
		return fmt.Errorf("failed to save user subscriptions: %w", err)
	}

	s.logger.Info("user subscription sync completed successfully",
		"user_id", user.UserID,
		"tenant_id", user.TenantID,
		"subscriptions_synced", len(subscriptions))

	return nil
}

func (s *userSyncService) GetUserSubscriptions(ctx context.Context) ([]Subscription, error) {
	// コンテキストからユーザー情報を取得
	user, ok := ctx.Value(middleware.UserContextKey).(*auth.UserContext)
	if !ok || user == nil {
		s.logger.Error("user context not found")
		return nil, fmt.Errorf("authentication required: user context not found")
	}

	s.logger.Info("retrieving user subscriptions",
		"user_id", user.UserID,
		"tenant_id", user.TenantID)

	// ユーザー固有のサブスクリプションを取得
	subscriptions, err := s.inoreaderClient.GetUserSubscriptions(ctx, user.UserID.String())
	if err != nil {
		s.logger.Error("failed to get user subscriptions",
			"error", err,
			"user_id", user.UserID)
		return nil, fmt.Errorf("failed to get user subscriptions: %w", err)
	}

	s.logger.Info("user subscriptions retrieved successfully",
		"user_id", user.UserID,
		"subscription_count", len(subscriptions))

	return subscriptions, nil
}
