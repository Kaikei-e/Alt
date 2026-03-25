package alt_db

// KnowledgeRepository handles all knowledge backfill database operations.
type KnowledgeRepository struct {
	pool PgxIface
}

func NewKnowledgeRepository(pool PgxIface) *KnowledgeRepository {
	if pool == nil {
		return nil
	}
	return &KnowledgeRepository{pool: pool}
}
