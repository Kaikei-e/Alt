package alt_db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxIface defines the interface for pgx operations that we use
type PgxIface interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Close()
}

// Ensure pgxpool.Pool implements PgxIface
var _ PgxIface = (*pgxpool.Pool)(nil)

// AltDBRepository is a backward-compatible facade that composes all domain-specific
// repositories. New code should depend on the domain repositories directly
// (e.g., *FeedRepository, *ArticleRepository) rather than this facade.
type AltDBRepository struct {
	pool PgxIface

	// Domain-specific repositories (embedded for method promotion)
	*FeedRepository
	*ArticleRepository
	*TagRepository
	*ScrapingRepository
	*ImageRepository
	*RecapRepository
	*SubscriptionRepository
	*InternalRepository
	*SummaryRepository
	*KnowledgeRepository
	*OutboxRepository
	*DashboardRepository
	*TenantRepository
}

func NewAltDBRepository(pool PgxIface) *AltDBRepository {
	if pool == nil {
		return nil
	}
	return &AltDBRepository{
		pool:                   pool,
		FeedRepository:         NewFeedRepository(pool),
		ArticleRepository:      NewArticleRepository(pool),
		TagRepository:          NewTagRepository(pool),
		ScrapingRepository:     NewScrapingRepository(pool),
		ImageRepository:        NewImageRepository(pool),
		RecapRepository:        NewRecapRepository(pool),
		SubscriptionRepository: NewSubscriptionRepository(pool),
		InternalRepository:     NewInternalRepository(pool),
		SummaryRepository:      NewSummaryRepository(pool),
		KnowledgeRepository:    NewKnowledgeRepository(pool),
		OutboxRepository:       NewOutboxRepository(pool),
		DashboardRepository:    NewDashboardRepository(pool),
		TenantRepository:       NewTenantRepository(pool),
	}
}

// NewAltDBRepositoryWithPool creates a repository with a concrete pgxpool.Pool
// Returns nil if pool is nil, which should be handled by the caller
func NewAltDBRepositoryWithPool(pool *pgxpool.Pool) *AltDBRepository {
	if pool == nil {
		return nil
	}
	return NewAltDBRepository(pool)
}

// NewAltDBRepositoryForTest creates an AltDBRepository with nil pool but non-nil domain
// repositories. Used in tests that need a valid struct to test error paths.
func NewAltDBRepositoryForTest() *AltDBRepository {
	return &AltDBRepository{
		FeedRepository:         &FeedRepository{},
		ArticleRepository:      &ArticleRepository{},
		TagRepository:          &TagRepository{},
		ScrapingRepository:     &ScrapingRepository{},
		ImageRepository:        &ImageRepository{},
		RecapRepository:        &RecapRepository{},
		SubscriptionRepository: &SubscriptionRepository{},
		InternalRepository:     &InternalRepository{},
		SummaryRepository:      &SummaryRepository{},
		KnowledgeRepository:    &KnowledgeRepository{},
		OutboxRepository:       &OutboxRepository{},
		DashboardRepository:    &DashboardRepository{},
		TenantRepository:       &TenantRepository{},
	}
}

// GetPool returns the underlying PgxIface
func (r *AltDBRepository) GetPool() PgxIface {
	return r.pool
}
