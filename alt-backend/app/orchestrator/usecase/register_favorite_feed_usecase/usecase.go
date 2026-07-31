package register_favorite_feed_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/register_favorite_feed_port"
	"context"
	"errors"
	"fmt"
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
	// The authenticated user is resolved here, not below: past this point
	// the call leaves the process that knows who is logged in.
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return fmt.Errorf("authentication required: %w", err)
	}
	return u.gateway.RegisterFavoriteFeed(ctx, url, user.UserID)
}
