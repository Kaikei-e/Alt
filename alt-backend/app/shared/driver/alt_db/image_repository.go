package alt_db

// ImageRepository handles all image proxy and OGP database operations.
type ImageRepository struct {
	pool PgxIface
}

func NewImageRepository(pool PgxIface) *ImageRepository {
	if pool == nil {
		return nil
	}
	return &ImageRepository{pool: pool}
}
