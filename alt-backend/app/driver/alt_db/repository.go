package alt_db

import (
	"github.com/jackc/pgx/v5"
)

type AltDBRepository struct {
	db *pgx.Conn
}

func NewAltDBRepository(db *pgx.Conn) *AltDBRepository {
	return &AltDBRepository{db: db}
}
