package alt_db

// TenantRepository handles multi-tenant database operations.
type TenantRepository struct {
	pool PgxIface
}

func NewTenantRepository(pool PgxIface) *TenantRepository {
	if pool == nil {
		return nil
	}
	return &TenantRepository{pool: pool}
}
