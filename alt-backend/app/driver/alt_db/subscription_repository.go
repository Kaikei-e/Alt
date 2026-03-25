package alt_db

// SubscriptionRepository handles all subscription database operations.
type SubscriptionRepository struct {
	pool PgxIface
}

func NewSubscriptionRepository(pool PgxIface) *SubscriptionRepository {
	if pool == nil {
		return nil
	}
	return &SubscriptionRepository{pool: pool}
}
