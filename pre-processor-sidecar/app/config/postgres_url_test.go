package config

import (
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDatabaseConfig_PostgresURL_EscapesPassword(t *testing.T) {
	db := DatabaseConfig{
		Host:     "db.example",
		Port:     "5432",
		Name:     "pre_processor",
		User:     "pp_user",
		Password: `p@ss word"quote`,
		SSLMode:  "disable",
	}

	conn := db.PostgresURL()
	u, err := url.Parse(conn)
	if err != nil {
		t.Fatalf("PostgresURL not parseable as URL: %v", err)
	}
	pass, ok := u.User.Password()
	if !ok {
		t.Fatal("expected password in URL")
	}
	if u.User.Username() != "pp_user" {
		t.Fatalf("unexpected user: %q", u.User.Username())
	}
	if pass != `p@ss word"quote` {
		t.Fatalf("password not round-tripped, got %q", pass)
	}

	if _, err := pgxpool.ParseConfig(conn); err != nil {
		t.Fatalf("pgxpool.ParseConfig rejected URL: %v", err)
	}
}
