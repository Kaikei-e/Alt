package alt_db

import (
	"database/sql"
)

type AltDBRepository struct {
	db *sql.DB
}