package register_favorite_feed_usecase

import (
	"alt/port/register_favorite_feed_port"
	"context"
	"errors"
	"strings"
)

type RegisterFavoriteFeedUsecase struct {
	gateway register_favorite_feed_port.RegisterFavoriteFeedPort
}

func NewRegisterFavoriteFeedUsecase(g register_favorite_feed_port.RegisterFavoriteFeedPort) *RegisterFavoriteFeedUsecase {
	return &RegisterFavoriteFeedUsecase{gateway: g}
}

func (u *RegisterFavoriteFeedUsecase) Execute(ctx context.Context, url string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("feed url cannot be empty")
	}
	return u.gateway.RegisterFavoriteFeed(ctx, url)
}
