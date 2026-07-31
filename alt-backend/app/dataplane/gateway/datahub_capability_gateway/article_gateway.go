package datahub_capability_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	"alt/shared/driver/alt_db"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// §2.B Article write
// ---------------------------------------------------------------------------

// articleWriteDriver is the slice of alt_db the archive capability uses.
//
// One method, and it is the transaction: SaveArticleForUser holds the articles
// upsert and the outbox insert together. Nothing here can enqueue an event on
// its own, which is the invariant the RPC boundary is drawn around.
type articleWriteDriver interface {
	SaveArticleForUser(ctx context.Context, url, title, content string, userID uuid.UUID) (string, bool, error)
}

// ArticleWriteGateway implements datahub_capability_port.ArticleWritePort.
type ArticleWriteGateway struct {
	db articleWriteDriver
}

func NewArticleWriteGateway(db *alt_db.AltDBRepository) *ArticleWriteGateway {
	return &ArticleWriteGateway{db: db}
}

func (g *ArticleWriteGateway) Archive(ctx context.Context, url, title, content string, userID uuid.UUID) (string, bool, error) {
	articleID, created, err := g.db.SaveArticleForUser(ctx, url, title, content, userID)
	if err != nil {
		return "", false, fmt.Errorf("archive article %s: %w", url, err)
	}
	return articleID, created, nil
}

// ---------------------------------------------------------------------------
// §2.C Article reads
// ---------------------------------------------------------------------------

type articleReadDriver interface {
	FetchArticleByURLForUser(ctx context.Context, articleURL string, userID *uuid.UUID) (*domain.ArticleContent, error)
	FetchArticlesByURLsForUser(ctx context.Context, urls []string, userID *uuid.UUID) (map[string]*domain.ArticleContent, error)
	FetchArticleByID(ctx context.Context, articleID string) (*domain.ArticleContent, error)
	FetchArticlesWithCursorForUser(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]*domain.Article, error)
	FetchArticleIDsWithCursorForUser(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]uuid.UUID, error)
	FetchArticlesByIDs(ctx context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error)
	FetchLatestArticleByFeedID(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error)
	LookupArticleURL(ctx context.Context, articleID string, userID uuid.UUID) (string, error)
}

// ArticleReadGateway implements datahub_capability_port.ArticleReadPort.
type ArticleReadGateway struct {
	db articleReadDriver
}

func NewArticleReadGateway(db *alt_db.AltDBRepository) *ArticleReadGateway {
	return &ArticleReadGateway{db: db}
}

func (g *ArticleReadGateway) GetByURL(ctx context.Context, url string, userID *uuid.UUID) (*domain.ArticleContent, error) {
	article, err := g.db.FetchArticleByURLForUser(ctx, url, userID)
	if err != nil {
		return nil, fmt.Errorf("get article by url: %w", err)
	}
	return article, nil
}

func (g *ArticleReadGateway) BatchGetByURLs(ctx context.Context, urls []string, userID *uuid.UUID) (map[string]*domain.ArticleContent, error) {
	articles, err := g.db.FetchArticlesByURLsForUser(ctx, urls, userID)
	if err != nil {
		return nil, fmt.Errorf("batch get articles by urls (%d): %w", len(urls), err)
	}
	return articles, nil
}

func (g *ArticleReadGateway) GetContentByID(ctx context.Context, articleID string) (*domain.ArticleContent, error) {
	article, err := g.db.FetchArticleByID(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("get article content %s: %w", articleID, err)
	}
	return article, nil
}

func (g *ArticleReadGateway) ListCursor(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]*domain.Article, error) {
	articles, err := g.db.FetchArticlesWithCursorForUser(ctx, userID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list articles cursor: %w", err)
	}
	return articles, nil
}

func (g *ArticleReadGateway) ListIDsCursor(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]uuid.UUID, error) {
	ids, err := g.db.FetchArticleIDsWithCursorForUser(ctx, userID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list article ids cursor: %w", err)
	}
	return ids, nil
}

func (g *ArticleReadGateway) BatchGetByIDs(ctx context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error) {
	articles, err := g.db.FetchArticlesByIDs(ctx, articleIDs)
	if err != nil {
		return nil, fmt.Errorf("batch get articles by ids (%d): %w", len(articleIDs), err)
	}
	return articles, nil
}

func (g *ArticleReadGateway) GetLatestByFeedID(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error) {
	article, err := g.db.FetchLatestArticleByFeedID(ctx, feedID)
	if err != nil {
		return nil, fmt.Errorf("get latest article for feed %s: %w", feedID, err)
	}
	return article, nil
}

func (g *ArticleReadGateway) LookupURL(ctx context.Context, articleID string, userID uuid.UUID) (string, error) {
	url, err := g.db.LookupArticleURL(ctx, articleID, userID)
	if err != nil {
		return "", fmt.Errorf("lookup article url %s: %w", articleID, err)
	}
	return url, nil
}

// ---------------------------------------------------------------------------
// §2.N Knowledge backfill reads
// ---------------------------------------------------------------------------

type knowledgeBackfillDriver interface {
	CountBackfillArticles(ctx context.Context) (int, error)
	ListBackfillArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error)
	CountBackfillSummaryTitles(ctx context.Context) (int, error)
	ListBackfillSummaryTitles(ctx context.Context, lastGeneratedAt *time.Time, lastSummaryVersionID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillSummaryTitle, error)
}

// KnowledgeBackfillGateway implements
// datahub_capability_port.KnowledgeBackfillPort.
type KnowledgeBackfillGateway struct {
	db knowledgeBackfillDriver
}

func NewKnowledgeBackfillGateway(db *alt_db.AltDBRepository) *KnowledgeBackfillGateway {
	return &KnowledgeBackfillGateway{db: db}
}

func (g *KnowledgeBackfillGateway) CountArticles(ctx context.Context) (int, error) {
	count, err := g.db.CountBackfillArticles(ctx)
	if err != nil {
		return 0, fmt.Errorf("count backfill articles: %w", err)
	}
	return count, nil
}

func (g *KnowledgeBackfillGateway) ListArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error) {
	articles, err := g.db.ListBackfillArticles(ctx, lastCreatedAt, lastArticleID, limit)
	if err != nil {
		return nil, fmt.Errorf("list backfill articles: %w", err)
	}
	return articles, nil
}

func (g *KnowledgeBackfillGateway) CountSummaryTitles(ctx context.Context) (int, error) {
	count, err := g.db.CountBackfillSummaryTitles(ctx)
	if err != nil {
		return 0, fmt.Errorf("count backfill summary titles: %w", err)
	}
	return count, nil
}

func (g *KnowledgeBackfillGateway) ListSummaryTitles(ctx context.Context, lastGeneratedAt *time.Time, lastSummaryVersionID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillSummaryTitle, error) {
	entries, err := g.db.ListBackfillSummaryTitles(ctx, lastGeneratedAt, lastSummaryVersionID, limit)
	if err != nil {
		return nil, fmt.Errorf("list backfill summary titles: %w", err)
	}
	return entries, nil
}
