package register_favorite_feed_port

import (
	"context"

	"github.com/google/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -source=register_port.go -destination=../../mocks/mock_register_favorite_feed_port.go -package=mocks RegisterFavoriteFeedPort

// RegisterFavoriteFeedPort stars and unstars feeds for one user.
//
// userID is explicit for the same reason as UpdateArticleStatusPort: the write
// is served by alt-data-hub, where there is no request context to read a
// reader out of.
type RegisterFavoriteFeedPort interface {
	RegisterFavoriteFeed(ctx context.Context, url string, userID uuid.UUID) error
	RemoveFavoriteFeed(ctx context.Context, url string, userID uuid.UUID) error
}
