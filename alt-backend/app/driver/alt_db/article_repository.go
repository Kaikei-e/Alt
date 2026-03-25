package alt_db

// ArticleRepository handles all article-related database operations.
type ArticleRepository struct {
	pool PgxIface
}

// NewArticleRepository creates a new ArticleRepository.
func NewArticleRepository(pool PgxIface) *ArticleRepository {
	if pool == nil {
		return nil
	}
	return &ArticleRepository{pool: pool}
}
