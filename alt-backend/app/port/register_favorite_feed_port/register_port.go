package register_favorite_feed_port

import "context"

//go:generate go run go.uber.org/mock/mockgen -source=register_port.go -destination=../../mocks/mock_register_favorite_feed_port.go -package=mocks RegisterFavoriteFeedPort

type RegisterFavoriteFeedPort interface {
	RegisterFavoriteFeed(ctx context.Context, url string) error
}
