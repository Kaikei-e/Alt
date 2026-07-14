package alt_db

// FeedRepository handles all feed-related database operations.
type FeedRepository struct {
	pool PgxIface
}

// NewFeedRepository creates a new FeedRepository.
func NewFeedRepository(pool PgxIface) *FeedRepository {
	if pool == nil {
		return nil
	}
	return &FeedRepository{pool: pool}
}

// GetPool returns the underlying PgxIface for backward compatibility.
func (r *FeedRepository) GetPool() PgxIface {
	return r.pool
}
