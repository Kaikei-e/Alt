package alt_db

// PushRepository owns the two Web Push tables: push_subscriptions (one row per
// device) and push_deliveries (the dispatcher's queue, one row per
// notification per device).
//
// Both live here rather than in two repositories because they are one
// capability read from two angles — a subscription is where a notification
// goes — and because the enqueue fan-out reads one to write the other.
type PushRepository struct {
	pool PgxIface
}

func NewPushRepository(pool PgxIface) *PushRepository {
	if pool == nil {
		return nil
	}
	return &PushRepository{pool: pool}
}
