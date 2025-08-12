// ABOUTME: This file handles user-specific subscription synchronization
// ABOUTME: It processes user data with proper tenant isolation and authentication

package service

import (
	"context"
	"fmt"
	"log/slog"

	"alt/shared/auth-lib-go/pkg/auth"
)

type UserSyncService struct {
	inoreaderClient InoreaderClient
	dbRepository    Repository
	logger          *slog.Logger
}

type InoreaderClient interface {
	GetUserSubscriptions(ctx context.Context, userID string) ([]Subscription, error)
}

type Repository interface {
	SaveUserSubscriptions(ctx context.Context, tenantID, userID string, subscriptions []Subscription) error
}

type Subscription struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func NewUserSyncService(
	inoreaderClient InoreaderClient,
	dbRepository Repository,
	logger *slog.Logger,
) *UserSyncService {
	return &UserSyncService{
		inoreaderClient: inoreaderClient,
		dbRepository:    dbRepository,
		logger:          logger,
	}
}

func (s *UserSyncService) SyncUserSubscriptions(ctx context.Context) error {
	// コンテキストからユーザー情報を取得
	user, ok := ctx.Value("user").(*auth.UserContext)
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

func (s *UserSyncService) GetUserSubscriptions(ctx context.Context) ([]Subscription, error) {
	// コンテキストからユーザー情報を取得
	user, ok := ctx.Value("user").(*auth.UserContext)
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