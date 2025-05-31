package alt_db

import (
	"database/sql"
)

type AltDBRepository struct {
	db *sql.DB
}

func NewAltDBRepository(db *sql.DB) *AltDBRepository {
	return &AltDBRepository{db: db}
}
