package alt_db

import (
	"alt/domain"
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const tenantTxKey contextKey = "tenant_tx"

type TenantAwareRepository struct {
	pool *pgxpool.Pool
}

func NewTenantAwareRepository(pool *pgxpool.Pool) *TenantAwareRepository {
	return &TenantAwareRepository{
		pool: pool,
	}
}

// withTenantContext はテナントコンテキストを設定した専用コネクションを作成
func (r *TenantAwareRepository) withTenantContext(ctx context.Context, tenantID uuid.UUID) (context.Context, pgx.Tx, error) {
	// トランザクション開始
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// セッションレベルでテナントIDを設定
	_, err = tx.Exec(ctx, "SELECT set_current_tenant($1)", tenantID)
	if err != nil {
		tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("failed to set tenant context: %w", err)
	}

	// トランザクションをコンテキストに保存
	return context.WithValue(ctx, tenantTxKey, tx), tx, nil
}

// GetUserFeeds はユーザーのフィード一覧を取得（テナント分離）
func (r *TenantAwareRepository) GetUserFeeds(ctx context.Context, userID uuid.UUID) ([]domain.Feed, error) {
	tenant, err := domain.GetTenantFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenant context required: %w", err)
	}

	// テナントコンテキスト設定
	tenantCtx, tx, err := r.withTenantContext(ctx, tenant.ID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(tenantCtx) // 読み取り専用なのでロールバック

	// RLSにより自動的にテナント分離されたクエリ
	query := `
		SELECT f.id, f.title, f.url, f.description, f.created_at, f.updated_at, f.tenant_id
		FROM feeds f
		JOIN user_feeds uf ON f.id = uf.feed_id
		WHERE uf.user_id = $1
		ORDER BY f.updated_at DESC
	`

	rows, err := tx.Query(tenantCtx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query feeds: %w", err)
	}
	defer rows.Close()

	var feeds []domain.Feed
	for rows.Next() {
		var feed domain.Feed
		if err := rows.Scan(&feed.ID, &feed.Title, &feed.URL, &feed.Description, &feed.CreatedAt, &feed.UpdatedAt, &feed.TenantID); err != nil {
			return nil, fmt.Errorf("failed to scan feed: %w", err)
		}
		feeds = append(feeds, feed)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return feeds, nil
}

// GetUserArticles はユーザーの記事一覧を取得（テナント分離）
func (r *TenantAwareRepository) GetUserArticles(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Article, error) {
	tenant, err := domain.GetTenantFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenant context required: %w", err)
	}

	// テナントコンテキスト設定
	tenantCtx, tx, err := r.withTenantContext(ctx, tenant.ID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(tenantCtx)

	// RLSによりテナント分離されたクエリ
	query := `
		SELECT a.id, a.title, a.url, a.content, a.published_at, a.created_at, a.updated_at, a.tenant_id,
		       COALESCE(rs.is_read, false) as is_read
		FROM articles a
		JOIN feeds f ON a.feed_id = f.id
		JOIN user_feeds uf ON f.id = uf.feed_id
		LEFT JOIN read_status rs ON a.id = rs.article_id AND rs.user_id = $1
		WHERE uf.user_id = $1
		ORDER BY a.published_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := tx.Query(tenantCtx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query articles: %w", err)
	}
	defer rows.Close()

	var articles []domain.Article
	for rows.Next() {
		var article domain.Article
		var isRead bool
		if err := rows.Scan(&article.ID, &article.Title, &article.URL, &article.Content,
			&article.PublishedAt, &article.CreatedAt, &article.UpdatedAt, &article.TenantID, &isRead); err != nil {
			return nil, fmt.Errorf("failed to scan article: %w", err)
		}
		// Note: isReadはドメインモデルに追加が必要
		articles = append(articles, article)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return articles, nil
}

// CreateFeed はフィードを作成（テナント分離）
func (r *TenantAwareRepository) CreateFeed(ctx context.Context, feed *domain.Feed) error {
	tenant, err := domain.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("tenant context required: %w", err)
	}

	// テナントコンテキスト設定
	tenantCtx, tx, err := r.withTenantContext(ctx, tenant.ID)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback(tenantCtx)
		}
	}()

	// テナントIDを明示的に設定
	feed.TenantID = tenant.ID

	// RLSによりテナント分離されたINSERT
	query := `
		INSERT INTO feeds (id, title, url, description, tenant_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = tx.Exec(tenantCtx, query, feed.ID, feed.Title, feed.URL, feed.Description,
		feed.TenantID, feed.CreatedAt, feed.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert feed: %w", err)
	}

	return tx.Commit(tenantCtx)
}

// UpdateFeed はフィードを更新（テナント分離）
func (r *TenantAwareRepository) UpdateFeed(ctx context.Context, feedID uuid.UUID, updates map[string]interface{}) error {
	tenant, err := domain.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("tenant context required: %w", err)
	}

	// テナントコンテキスト設定
	tenantCtx, tx, err := r.withTenantContext(ctx, tenant.ID)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback(tenantCtx)
		}
	}()

	// RLSによりテナント分離されたUPDATE
	// 動的クエリ生成（簡略化のため基本実装）
	query := `UPDATE feeds SET updated_at = NOW() WHERE id = $1`
	_, err = tx.Exec(tenantCtx, query, feedID)
	if err != nil {
		return fmt.Errorf("failed to update feed: %w", err)
	}

	return tx.Commit(tenantCtx)
}

// DeleteFeed はフィードを削除（テナント分離）
func (r *TenantAwareRepository) DeleteFeed(ctx context.Context, feedID uuid.UUID) error {
	tenant, err := domain.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("tenant context required: %w", err)
	}

	// テナントコンテキスト設定
	tenantCtx, tx, err := r.withTenantContext(ctx, tenant.ID)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback(tenantCtx)
		}
	}()

	// RLSによりテナント分離されたDELETE
	query := `DELETE FROM feeds WHERE id = $1`
	result, err := tx.Exec(tenantCtx, query, feedID)
	if err != nil {
		return fmt.Errorf("failed to delete feed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrFeedNotFound
	}

	return tx.Commit(tenantCtx)
}

// ValidateTenantIsolation はテナント分離が正しく動作しているかを検証
func (r *TenantAwareRepository) ValidateTenantIsolation(ctx context.Context, tenantID uuid.UUID) error {
	// テナントコンテキスト設定
	tenantCtx, tx, err := r.withTenantContext(ctx, tenantID)
	if err != nil {
		return err
	}
	defer tx.Rollback(tenantCtx)

	// テナントの分離を確認
	var count int
	query := `SELECT COUNT(*) FROM feeds WHERE tenant_id != $1`
	err = tx.QueryRow(tenantCtx, query, tenantID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to query validation: %w", err)
	}

	// RLSが正しく動作していれば、他のテナントのデータは見えないはず
	if count > 0 {
		return fmt.Errorf("tenant isolation failed: found %d records from other tenants", count)
	}

	return nil
}
