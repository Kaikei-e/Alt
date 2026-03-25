package alt_db

// DashboardRepository handles all dashboard and trend stats database operations.
type DashboardRepository struct {
	pool PgxIface
}

func NewDashboardRepository(pool PgxIface) *DashboardRepository {
	if pool == nil {
		return nil
	}
	return &DashboardRepository{pool: pool}
}
