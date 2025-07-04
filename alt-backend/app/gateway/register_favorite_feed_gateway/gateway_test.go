package register_favorite_feed_gateway

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRegisterFavoriteFeedGateway_RegisterFavoriteFeed(t *testing.T) {
	g := &RegisterFavoriteFeedGateway{alt_db: nil}

	if err := g.RegisterFavoriteFeed(context.Background(), "https://example.com"); err == nil {
		t.Errorf("expected error when db is nil")
	}
}

func TestNewRegisterFavoriteFeedGateway(t *testing.T) {
	var pool *pgxpool.Pool
	g := NewRegisterFavoriteFeedGateway(pool)
	if g == nil {
		t.Fatal("gateway is nil")
	}
	if g.alt_db != nil {
		t.Fatal("repository should be nil when pool is nil")
	}
}
