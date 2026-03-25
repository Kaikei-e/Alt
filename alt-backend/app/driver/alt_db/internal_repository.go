package alt_db

// InternalRepository handles all internal service API database operations.
type InternalRepository struct {
	pool PgxIface
}

func NewInternalRepository(pool PgxIface) *InternalRepository {
	if pool == nil {
		return nil
	}
	return &InternalRepository{pool: pool}
}
