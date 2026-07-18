package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsDuplicateError_PgErrorCode(t *testing.T) {
	r := &PostgreSQLArticleRepository{}
	err := &pgconn.PgError{Code: "23505", Message: "duplicate key"}
	if !r.isDuplicateError(err) {
		t.Fatal("expected 23505 to be duplicate")
	}
	if !r.isDuplicateError(fmt.Errorf("wrapped: %w", err)) {
		t.Fatal("expected wrapped 23505 to be duplicate")
	}
	if r.isDuplicateError(errors.New("duplicate key value violates unique constraint")) {
		t.Fatal("string-only match must not succeed; require PgError")
	}
}

func TestIsForeignKeyError_PgErrorCode(t *testing.T) {
	r := &PostgreSQLArticleRepository{}
	err := &pgconn.PgError{Code: "23503", Message: "fk violation"}
	if !r.isForeignKeyError(err) {
		t.Fatal("expected 23503 to be foreign key")
	}
	if r.isForeignKeyError(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("23505 must not be foreign key")
	}
}
