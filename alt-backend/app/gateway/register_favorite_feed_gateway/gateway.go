package register_favorite_feed_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
	urlpkg "net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RegisterFavoriteFeedGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewRegisterFavoriteFeedGateway(pool *pgxpool.Pool) *RegisterFavoriteFeedGateway {
	return &RegisterFavoriteFeedGateway{alt_db: alt_db.NewAltDBRepositoryWithPool(pool)}
}

func (g *RegisterFavoriteFeedGateway) RegisterFavoriteFeed(ctx context.Context, url string) error {
	if g.alt_db == nil {
		return errors.New("database connection not available")
	}
	parsed, err := urlpkg.Parse(url)
	if err != nil || parsed.Scheme == "" {
		return errors.New("invalid URL format")
	}
	err = g.alt_db.RegisterFavoriteFeed(ctx, strings.TrimSpace(url))
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return errors.New("feed not found")
		case errors.Is(err, pgx.ErrTxClosed):
			logger.SafeError("failed to register favorite feed", "error", err)
			return errors.New("failed to register favorite feed")
		default:
			logger.SafeError("error inserting favorite feed", "error", err)
			return err
		}
	}
	logger.SafeInfo("favorite feed registered", "url", url)
	return nil
}
