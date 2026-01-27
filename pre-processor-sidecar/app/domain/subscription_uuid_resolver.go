// ABOUTME: SubscriptionUUIDResolver - UUID解決のコアドメインサービス
// ABOUTME: 単一責任の原則に従い、各処理を独立した関数に分割

package domain

import (
	"context"

	"pre-processor-sidecar/models"

	"github.com/google/uuid"
)

// SubscriptionUUIDResolver はUUID解決の業務ロジックを担当
type SubscriptionUUIDResolver struct {
	autoCreator SubscriptionAutoCreator
	logger      LoggerInterface
}

// NewSubscriptionUUIDResolver は新しいリゾルバーを作成
func NewSubscriptionUUIDResolver(
	autoCreator SubscriptionAutoCreator,
	logger LoggerInterface,
) *SubscriptionUUIDResolver {
	return &SubscriptionUUIDResolver{
		autoCreator: autoCreator,
		logger:      logger,
	}
}

// ResolveArticleUUIDs は記事群のUUID解決を実行（メイン調整関数）
func (r *SubscriptionUUIDResolver) ResolveArticleUUIDs(
	ctx context.Context,
	articles []*models.Article,
	mapping *SubscriptionMapping,
) (*UUIDResolutionResult, error) {
	r.logger.Info("Starting subscription UUID resolution",
		"total_articles", len(articles),
		"mapping_size", mapping.Size())

	result := &UUIDResolutionResult{
		TotalProcessed: len(articles),
		Errors:         make([]ResolutionError, 0),
	}

	// 各記事を処理
	for i, article := range articles {
		r.logger.Debug("Processing article for UUID resolution",
			"article_index", i+1,
			"inoreader_id", article.InoreaderID,
			"origin_stream_id", article.OriginStreamID,
			"current_subscription_id", article.SubscriptionID)

		// 1. バリデーション
		if err := r.validateArticleForResolution(article); err != nil {
			r.addResolutionError(result, article, err, "VALIDATION_ERROR")
			result.UnknownCount++
			continue
		}

		// 2. 既知のサブスクリプション解決を試行
		if subscriptionUUID, found := r.resolveKnownSubscription(article.OriginStreamID, mapping); found {
			r.updateArticleSubscriptionID(article, subscriptionUUID)
			result.ResolvedCount++
			r.logger.Debug("Found cached subscription UUID",
				"origin_stream_id", article.OriginStreamID,
				"resolved_uuid", subscriptionUUID)
			continue
		}

		// 3. 未知サブスクリプションの自動作成
		r.logger.Warn("Unknown subscription detected, attempting auto-creation",
			"article_inoreader_id", article.InoreaderID,
			"origin_stream_id", article.OriginStreamID)

		newUUID, err := r.autoCreateMissingSubscription(ctx, article.OriginStreamID)
		if err != nil {
			r.addResolutionError(result, article, err, "AUTO_CREATION_ERROR")
			result.UnknownCount++
			r.updateArticleSubscriptionID(article, uuid.Nil) // 明示的にNil設定
		} else {
			r.updateArticleSubscriptionID(article, newUUID)
			result.AutoCreatedCount++

			// マッピングキャッシュを更新
			mapping.SetMapping(article.OriginStreamID, newUUID)
			r.logger.Info("Auto-created missing subscription",
				"origin_stream_id", article.OriginStreamID,
				"new_uuid", newUUID)
		}

		// 4. 最終検証ログ
		r.logger.Debug("Article UUID resolution completed",
			"article_index", i+1,
			"inoreader_id", article.InoreaderID,
			"final_subscription_id", article.SubscriptionID,
			"is_nil", article.SubscriptionID == uuid.Nil)
	}

	// 5. 重要: 全処理完了後に一時フィールドをクリア
	r.clearTemporaryFields(articles)

	r.logger.Info("Subscription UUID resolution completed",
		"total_articles", result.TotalProcessed,
		"resolved", result.ResolvedCount,
		"auto_created", result.AutoCreatedCount,
		"unknown", result.UnknownCount,
		"errors", len(result.Errors))

	return result, nil
}

// validateArticleForResolution は記事のUUID解決バリデーション
func (r *SubscriptionUUIDResolver) validateArticleForResolution(article *models.Article) error {
	if article == nil {
		return ErrInvalidArticle
	}
	if article.OriginStreamID == "" {
		return ErrEmptyOriginStreamID
	}
	return nil
}

// resolveKnownSubscription は既知サブスクリプションのUUID解決
func (r *SubscriptionUUIDResolver) resolveKnownSubscription(
	originStreamID string,
	mapping *SubscriptionMapping,
) (uuid.UUID, bool) {
	return mapping.GetUUID(originStreamID)
}

// autoCreateMissingSubscription は未知サブスクリプションの自動作成
func (r *SubscriptionUUIDResolver) autoCreateMissingSubscription(
	ctx context.Context,
	originStreamID string,
) (uuid.UUID, error) {
	if r.autoCreator == nil {
		return uuid.Nil, ErrAutoCreationFailed
	}
	return r.autoCreator.AutoCreateSubscription(ctx, originStreamID)
}

// updateArticleSubscriptionID は記事のSubscriptionIDを更新
func (r *SubscriptionUUIDResolver) updateArticleSubscriptionID(
	article *models.Article,
	subscriptionID uuid.UUID,
) {
	article.SubscriptionID = subscriptionID
}

// clearTemporaryFields は全記事の一時フィールドをクリア（重要: 最後に実行）
func (r *SubscriptionUUIDResolver) clearTemporaryFields(articles []*models.Article) {
	r.logger.Debug("Clearing temporary fields for all articles", "count", len(articles))
	for _, article := range articles {
		article.OriginStreamID = ""
	}
}

// addResolutionError はエラーを結果に追加
func (r *SubscriptionUUIDResolver) addResolutionError(
	result *UUIDResolutionResult,
	article *models.Article,
	err error,
	errorCode string,
) {
	resolutionError := ResolutionError{
		ArticleInoreaderID: article.InoreaderID,
		OriginStreamID:     article.OriginStreamID,
		ErrorMessage:       err.Error(),
		ErrorCode:          errorCode,
	}
	result.Errors = append(result.Errors, resolutionError)

	r.logger.Error("Failed to resolve article UUID",
		"inoreader_id", article.InoreaderID,
		"origin_stream_id", article.OriginStreamID,
		"error", err,
		"error_code", errorCode)
}
