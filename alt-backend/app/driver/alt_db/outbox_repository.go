package alt_db

// OutboxRepository handles all outbox event database operations.
type OutboxRepository struct {
	pool PgxIface
}

func NewOutboxRepository(pool PgxIface) *OutboxRepository {
	if pool == nil {
		return nil
	}
	return &OutboxRepository{pool: pool}
}
