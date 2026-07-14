package alt_db

// RecapRepository handles all recap/digest database operations.
type RecapRepository struct {
	pool PgxIface
}

func NewRecapRepository(pool PgxIface) *RecapRepository {
	if pool == nil {
		return nil
	}
	return &RecapRepository{pool: pool}
}
