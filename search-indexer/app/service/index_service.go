package service

import (
	"context"
	"fmt"
	"log/slog"

	"search-indexer/internal/auth"
)

type IndexService struct {
	meilisearchClient MeilisearchClient
	logger            *slog.Logger
}

type MeilisearchClient interface {
	Index(indexName string, article Article) error
	Search(indexName string, query SearchQuery) (*SearchResult, error)
}

type Article struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
}

type SearchQuery struct {
	Query   string            `json:"query"`
	Filters []string          `json:"filters"`
	Limit   int               `json:"limit"`
	Options map[string]string `json:"options"`
}

type SearchResult struct {
	Hits       []Article `json:"hits"`
	TotalHits  int       `json:"total_hits"`
	Query      string    `json:"query"`
	TimeTaken  int       `json:"time_taken_ms"`
}

func NewIndexService(meilisearchClient MeilisearchClient, logger *slog.Logger) *IndexService {
	return &IndexService{
		meilisearchClient: meilisearchClient,
		logger:            logger,
	}
}

func (s *IndexService) IndexUserArticle(ctx context.Context, article Article) error {
	user, ok := ctx.Value("user").(*auth.UserContext)
	if !ok || user == nil {
		s.logger.Error("user context not found")
		return fmt.Errorf("authentication required: user context not found")
	}

	// テナント固有のインデックス名
	indexName := fmt.Sprintf("articles_%s", user.TenantID)

	s.logger.Info("indexing user article",
		"article_id", article.ID,
		"user_id", user.UserID,
		"tenant_id", user.TenantID,
		"index_name", indexName)

	// ユーザー固有のメタデータを追加
	article.UserID = user.UserID.String()
	article.TenantID = user.TenantID.String()

	err := s.meilisearchClient.Index(indexName, article)
	if err != nil {
		s.logger.Error("failed to index article",
			"error", err,
			"article_id", article.ID,
			"user_id", user.UserID,
			"tenant_id", user.TenantID)
		return fmt.Errorf("failed to index article: %w", err)
	}

	s.logger.Info("article indexed successfully",
		"article_id", article.ID,
		"user_id", user.UserID,
		"tenant_id", user.TenantID)

	return nil
}

func (s *IndexService) SearchUserArticles(ctx context.Context, query SearchQuery) (*SearchResult, error) {
	user, ok := ctx.Value("user").(*auth.UserContext)
	if !ok || user == nil {
		s.logger.Error("user context not found")
		return nil, fmt.Errorf("authentication required: user context not found")
	}

	// テナント固有のインデックスで検索
	indexName := fmt.Sprintf("articles_%s", user.TenantID)

	s.logger.Info("searching user articles",
		"query", query.Query,
		"user_id", user.UserID,
		"tenant_id", user.TenantID,
		"index_name", indexName)

	// ユーザー固有のフィルタを追加
	userFilter := fmt.Sprintf("user_id = %s", user.UserID)
	query.Filters = append(query.Filters, userFilter)

	result, err := s.meilisearchClient.Search(indexName, query)
	if err != nil {
		s.logger.Error("failed to search articles",
			"error", err,
			"query", query.Query,
			"user_id", user.UserID,
			"tenant_id", user.TenantID)
		return nil, fmt.Errorf("failed to search articles: %w", err)
	}

	s.logger.Info("articles search completed",
		"query", query.Query,
		"hits_count", result.TotalHits,
		"user_id", user.UserID,
		"tenant_id", user.TenantID)

	return result, nil
}

// BulkIndexUserArticles handles bulk indexing with tenant isolation
func (s *IndexService) BulkIndexUserArticles(ctx context.Context, articles []Article) error {
	user, ok := ctx.Value("user").(*auth.UserContext)
	if !ok || user == nil {
		s.logger.Error("user context not found")
		return fmt.Errorf("authentication required: user context not found")
	}

	// テナント固有のインデックス名
	indexName := fmt.Sprintf("articles_%s", user.TenantID)

	s.logger.Info("bulk indexing user articles",
		"article_count", len(articles),
		"user_id", user.UserID,
		"tenant_id", user.TenantID,
		"index_name", indexName)

	// 各記事にユーザー固有のメタデータを追加
	for i := range articles {
		articles[i].UserID = user.UserID.String()
		articles[i].TenantID = user.TenantID.String()
	}

	// バルクインデックス処理（実装は meilisearchClient に依存）
	for _, article := range articles {
		if err := s.meilisearchClient.Index(indexName, article); err != nil {
			s.logger.Error("failed to index article in bulk operation",
				"error", err,
				"article_id", article.ID,
				"user_id", user.UserID,
				"tenant_id", user.TenantID)
			return fmt.Errorf("bulk index failed at article %s: %w", article.ID, err)
		}
	}

	s.logger.Info("bulk indexing completed successfully",
		"article_count", len(articles),
		"user_id", user.UserID,
		"tenant_id", user.TenantID)

	return nil
}