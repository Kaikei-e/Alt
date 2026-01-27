// ABOUTME: ArticleUUIDResolutionUseCase - 業務フローの調整役
// ABOUTME: ドメインサービスとリポジトリを統合し、UUID解決ワークフローを管理

package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"pre-processor-sidecar/domain"
	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"

	"github.com/google/uuid"
)

// ArticleUUIDResolutionUseCase はUUID解決の業務ワークフローを管理
type ArticleUUIDResolutionUseCase struct {
	resolver         *domain.SubscriptionUUIDResolver
	subscriptionRepo repository.SubscriptionRepository
	logger           domain.LoggerInterface
}

// NewArticleUUIDResolutionUseCase は新しいユースケースを作成
func NewArticleUUIDResolutionUseCase(
	resolver *domain.SubscriptionUUIDResolver,
	subscriptionRepo repository.SubscriptionRepository,
	logger domain.LoggerInterface,
) *ArticleUUIDResolutionUseCase {
	return &ArticleUUIDResolutionUseCase{
		resolver:         resolver,
		subscriptionRepo: subscriptionRepo,
		logger:           logger,
	}
}

// ResolveArticleUUIDs はサブスクリプションマッピングを構築し、記事のUUID解決を実行
func (uc *ArticleUUIDResolutionUseCase) ResolveArticleUUIDs(
	ctx context.Context,
	articles []*models.Article,
) (*domain.UUIDResolutionResult, error) {
	uc.logger.Info("Starting article UUID resolution use case",
		"total_articles", len(articles))

	startTime := time.Now()

	// 1. サブスクリプションマッピングを構築
	mapping, err := uc.buildSubscriptionMapping(ctx)
	if err != nil {
		uc.logger.Error("Failed to build subscription mapping", "error", err)
		return nil, fmt.Errorf("failed to build subscription mapping: %w", err)
	}

	// 2. ドメインサービスでUUID解決を実行
	result, err := uc.resolver.ResolveArticleUUIDs(ctx, articles, mapping)
	if err != nil {
		uc.logger.Error("Failed to resolve article UUIDs", "error", err)
		return nil, fmt.Errorf("failed to resolve article UUIDs: %w", err)
	}

	duration := time.Since(startTime)
	uc.logger.Info("Article UUID resolution use case completed",
		"duration_ms", duration.Milliseconds(),
		"resolved", result.ResolvedCount,
		"auto_created", result.AutoCreatedCount,
		"unknown", result.UnknownCount,
		"total_processed", result.TotalProcessed,
		"errors", len(result.Errors))

	return result, nil
}

// buildSubscriptionMapping はサブスクリプションのマッピングキャッシュを構築
func (uc *ArticleUUIDResolutionUseCase) buildSubscriptionMapping(ctx context.Context) (*domain.SubscriptionMapping, error) {
	uc.logger.Debug("Building subscription mapping cache")

	startTime := time.Now()

	// データベースから全サブスクリプションを取得
	subscriptions, err := uc.subscriptionRepo.GetAllSubscriptions(ctx)
	if err != nil {
		uc.logger.Error("Failed to fetch all subscriptions", "error", err)
		return nil, fmt.Errorf("failed to fetch subscriptions for mapping: %w", err)
	}

	// スレッドセーフなマッピングを作成
	mapping := domain.NewSubscriptionMapping()

	for _, subscription := range subscriptions {
		mapping.SetMapping(subscription.InoreaderID, subscription.DatabaseID)
	}

	uc.logger.Info("Subscription mapping cache built successfully",
		"subscription_count", mapping.Size(),
		"build_duration_ms", time.Since(startTime).Milliseconds())

	return mapping, nil
}

// SubscriptionAutoCreatorAdapter はサービス層の自動作成機能をドメイン層に適応
type SubscriptionAutoCreatorAdapter struct {
	subscriptionRepo repository.SubscriptionRepository
	logger           domain.LoggerInterface
}

// NewSubscriptionAutoCreatorAdapter は新しいアダプターを作成
func NewSubscriptionAutoCreatorAdapter(
	subscriptionRepo repository.SubscriptionRepository,
	logger domain.LoggerInterface,
) *SubscriptionAutoCreatorAdapter {
	return &SubscriptionAutoCreatorAdapter{
		subscriptionRepo: subscriptionRepo,
		logger:           logger,
	}
}

// AutoCreateSubscription は未知サブスクリプションを自動作成
func (a *SubscriptionAutoCreatorAdapter) AutoCreateSubscription(
	ctx context.Context,
	originStreamID string,
) (uuid.UUID, error) {
	a.logger.Info("Auto-creating missing subscription",
		"inoreader_id", originStreamID)

	// フィードURLを抽出
	feedURL := a.extractFeedURLFromInoreaderID(originStreamID)
	if feedURL == "" {
		return uuid.Nil, fmt.Errorf("failed to extract feed URL from inoreader_id: %s", originStreamID)
	}

	// 新しいサブスクリプションレコードを作成
	subscription := models.NewSubscription(
		originStreamID,
		feedURL,
		a.generateAutoTitle(feedURL),
		"Auto-Created",
	)

	// データベースに保存
	if err := a.subscriptionRepo.CreateSubscription(ctx, subscription); err != nil {
		return uuid.Nil, fmt.Errorf("failed to create auto subscription: %w", err)
	}

	a.logger.Info("Successfully auto-created subscription",
		"inoreader_id", originStreamID,
		"uuid", subscription.ID,
		"feed_url", feedURL,
		"title", subscription.Title)

	return subscription.ID, nil
}

// extractFeedURLFromInoreaderID はInoreader IDからフィードURLを抽出
func (a *SubscriptionAutoCreatorAdapter) extractFeedURLFromInoreaderID(inoreaderID string) string {
	// "feed/https://example.com/rss.xml" -> "https://example.com/rss.xml"
	if strings.HasPrefix(inoreaderID, "feed/") {
		return strings.TrimPrefix(inoreaderID, "feed/")
	}

	// すでにURLの形式の場合
	if strings.Contains(inoreaderID, "://") {
		return inoreaderID
	}

	// 有効なURLを抽出できない場合
	a.logger.Warn("Unable to extract feed URL from inoreader_id",
		"inoreader_id", inoreaderID)
	return ""
}

// generateAutoTitle は自動作成サブスクリプションの適切なタイトルを生成
func (a *SubscriptionAutoCreatorAdapter) generateAutoTitle(feedURL string) string {
	// URLからドメイン名を抽出して適切なデフォルトタイトルを生成
	if strings.Contains(feedURL, "://") {
		parts := strings.Split(feedURL, "://")
		if len(parts) > 1 {
			hostPart := strings.Split(parts[1], "/")[0]
			// 一般的なプレフィックスを削除
			hostPart = strings.TrimPrefix(hostPart, "www.")
			return fmt.Sprintf("Auto: %s", hostPart)
		}
	}

	// フォールバックタイトル
	return "Auto-Created Feed"
}
