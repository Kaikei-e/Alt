package feed_link_usecase

import (
	"alt/port/subscription_port"
	"context"

	"github.com/google/uuid"
)

type DeleteFeedLinkUsecase struct {
	port subscription_port.SubscriptionPort
}

func NewDeleteFeedLinkUsecase(port subscription_port.SubscriptionPort) *DeleteFeedLinkUsecase {
	return &DeleteFeedLinkUsecase{port: port}
}

func (u *DeleteFeedLinkUsecase) Execute(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	return u.port.Unsubscribe(ctx, userID, feedLinkID)
}
