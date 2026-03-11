package remove_favorite_feed_usecase

import (
	"alt/port/register_favorite_feed_port"
	"context"
	"errors"
	"strings"
)

type RemoveFavoriteFeedUsecase struct {
	gateway register_favorite_feed_port.RegisterFavoriteFeedPort
}

func NewRemoveFavoriteFeedUsecase(g register_favorite_feed_port.RegisterFavoriteFeedPort) *RemoveFavoriteFeedUsecase {
	return &RemoveFavoriteFeedUsecase{gateway: g}
}

func (u *RemoveFavoriteFeedUsecase) Execute(ctx context.Context, url string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("feed url cannot be empty")
	}
	return u.gateway.RemoveFavoriteFeed(ctx, url)
}
