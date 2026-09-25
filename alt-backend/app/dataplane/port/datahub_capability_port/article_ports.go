package datahub_capability_port

import (
	"context"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

// ArticleWritePort is the article archive (catalog §2.B).
//
// One method, because there is one write: the articles upsert and the outbox
// insert are a single transaction and the interface offers no way to do half
// of it. A port with separate "upsert" and "enqueue" methods would compile,
// and would let a caller commit an article nobody is ever told about.
type ArticleWritePort interface {
	// Archive upserts by (url, user_id) and appends the ARTICLE_UPSERT outbox
	// row in the same transaction. The bool reports whether the row was newly
	// inserted.
	Archive(ctx context.Context, url, title, content string, userID uuid.UUID) (articleID string, created bool, err error)
}

// ArticleReadPort is what alt-backend's article-serving surfaces read
// (catalog §2.C).
//
// userID is an argument on every method that has one rather than something
// read from the context: alt-data-hub serves these over Connect, where the
// peer certificate identifies alt-backend and not the person whose articles
// are being listed.
type ArticleReadPort interface {
	// GetByURL returns nil without error when the URL has not been archived.
	// A nil userID means the unscoped lookup, which is a different query and
	// not a default.
	GetByURL(ctx context.Context, url string, userID *uuid.UUID) (*domain.ArticleContent, error)
	// BatchGetByURLs omits URLs with no archived article.
	BatchGetByURLs(ctx context.Context, urls []string, userID *uuid.UUID) (map[string]*domain.ArticleContent, error)
	// GetContentByID returns nil without error for an unknown id.
	GetContentByID(ctx context.Context, articleID string) (*domain.ArticleContent, error)
	// ListCursor pages one user's articles newest first, with tags.
	ListCursor(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]*domain.Article, error)
	// ListIDsCursor is the same walk without the tag join or the bodies.
	ListIDsCursor(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]uuid.UUID, error)
	// BatchGetByIDs preserves the requested order and omits unknown ids.
	BatchGetByIDs(ctx context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error)
	// GetLatestByFeedID returns nil without error for a feed with no articles.
	GetLatestByFeedID(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error)
	// LookupSource returns the zero value for an article outside the user's
	// tenant, which is deliberately the same answer as for one that does not
	// exist.
	LookupSource(ctx context.Context, articleID string, userID uuid.UUID) (domain.ArticleSource, error)
}

// KnowledgeBackfillPort is the alt_db half of the knowledge backfill jobs
// (catalog §2.N).
//
// Only the reads. The sovereign append, the progress bookkeeping and the
// reprojection stay with alt-backend, because talking to another service is
// the caller's business (ADR-000954 D4).
type KnowledgeBackfillPort interface {
	CountArticles(ctx context.Context) (int, error)
	// ListArticles walks (created_at, article_id) ascending. Both cursor
	// parts are nil on the first page and both are set afterwards.
	ListArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error)
	CountSummaryTitles(ctx context.Context) (int, error)
	// ListSummaryTitles walks (generated_at, summary_version_id) ascending.
	ListSummaryTitles(ctx context.Context, lastGeneratedAt *time.Time, lastSummaryVersionID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillSummaryTitle, error)
}

// ArticleRefPort is the recall rail's projection fallback (catalog §2.C,
// ADR-000954 Wave 3 batch 6).
//
// It is separate from ArticleReadPort deliberately. That port answers "give me
// this article" and every method on it belongs to a surface that renders
// articles; this one answers "the projection is behind, what did this id used
// to be" and has exactly one caller. Folding it in would hand every article
// consumer a method that only makes sense during projection lag.
type ArticleRefPort interface {
	// ArticleRef returns title, url and published_at for one article. A nil
	// result is "no such article" and is not an error: the rail asks about
	// candidates whose article may legitimately have been deleted since.
	ArticleRef(ctx context.Context, articleID string) (*domain.ArticleRef, error)
}
