package alt_db

// ScrapingRepository handles all scraping domain database operations.
type ScrapingRepository struct {
	pool PgxIface
}

func NewScrapingRepository(pool PgxIface) *ScrapingRepository {
	if pool == nil {
		return nil
	}
	return &ScrapingRepository{pool: pool}
}
