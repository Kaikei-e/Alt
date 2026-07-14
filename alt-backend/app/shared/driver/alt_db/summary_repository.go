package alt_db

// SummaryRepository handles all summary version database operations.
type SummaryRepository struct {
	pool PgxIface
}

func NewSummaryRepository(pool PgxIface) *SummaryRepository {
	if pool == nil {
		return nil
	}
	return &SummaryRepository{pool: pool}
}
