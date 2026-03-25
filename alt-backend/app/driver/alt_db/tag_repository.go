package alt_db

// TagRepository handles all tag-related database operations.
type TagRepository struct {
	pool PgxIface
}

func NewTagRepository(pool PgxIface) *TagRepository {
	if pool == nil {
		return nil
	}
	return &TagRepository{pool: pool}
}
