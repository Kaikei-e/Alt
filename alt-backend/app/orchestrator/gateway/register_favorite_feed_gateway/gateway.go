package register_favorite_feed_gateway

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	urlpkg "net/url"
	"strings"

	"github.com/google/uuid"
)

// favoriteFeedStore is the alt-data-hub capability behind the two writes
// (capability catalog §2.I W3-I9 / W3-I10).
type favoriteFeedStore interface {
	RegisterFavoriteFeed(ctx context.Context, url string, userID uuid.UUID) error
	RemoveFavoriteFeed(ctx context.Context, url string, userID uuid.UUID) error
}

// RegisterFavoriteFeedGateway is what is left on this side after the write
// moved: URL validation and the error vocabulary the REST layer renders.
//
// Both are pure functions over a string and a sentinel, and ADR-000954 D4
// keeps them with the caller. What is gone is the transaction — the SELECT and
// the insert are one commit inside the provider, where they were always meant
// to be.
type RegisterFavoriteFeedGateway struct {
	store favoriteFeedStore
}

func NewRegisterFavoriteFeedGateway(store favoriteFeedStore) *RegisterFavoriteFeedGateway {
	if store == nil {
		panic("register_favorite_feed_gateway: a favorite feed store is required — " +
			"a nil one would make every star silently do nothing (see .claude/rules/di-wiring.md)")
	}
	return &RegisterFavoriteFeedGateway{store: store}
}

func (g *RegisterFavoriteFeedGateway) RegisterFavoriteFeed(ctx context.Context, url string, userID uuid.UUID) error {
	cleanURL, err := validatedURL(url)
	if err != nil {
		return err
	}

	if err := g.store.RegisterFavoriteFeed(ctx, cleanURL, userID); err != nil {
		if errors.Is(err, domain.ErrFeedNotFound) {
			return errors.New("feed not found")
		}
		logger.SafeErrorContext(ctx, "error inserting favorite feed", "error", err)
		return errors.New("failed to register favorite feed")
	}
	logger.SafeInfoContext(ctx, "favorite feed registered", "url", cleanURL)
	return nil
}

func (g *RegisterFavoriteFeedGateway) RemoveFavoriteFeed(ctx context.Context, url string, userID uuid.UUID) error {
	cleanURL, err := validatedURL(url)
	if err != nil {
		return err
	}

	if err := g.store.RemoveFavoriteFeed(ctx, cleanURL, userID); err != nil {
		// The provider answers NotFound for both "no such feed" and "the user
		// had not starred it". Collapsing them here is not a loss: the two were
		// already the same pgx.ErrNoRows before the split, and the screen shows
		// the same message for either.
		if errors.Is(err, domain.ErrFeedNotFound) {
			return errors.New("favorite feed not found")
		}
		logger.SafeErrorContext(ctx, "error removing favorite feed", "error", err)
		return errors.New("failed to remove favorite feed")
	}
	logger.SafeInfoContext(ctx, "favorite feed removed", "url", cleanURL)
	return nil
}

// validatedURL rejects what the provider would have no way to interpret. It
// stays on this side because it reads a string and returns a string — no
// database is involved in deciding that "not a url" is not a url.
func validatedURL(raw string) (string, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "", errors.New("empty url")
	}
	parsed, err := urlpkg.Parse(clean)
	if err != nil || parsed.Scheme == "" {
		return "", errors.New("invalid URL format")
	}
	return clean, nil
}
