package subscription_usecase

import (
	"alt/port/subscription_port"
	"context"

	"github.com/google/uuid"
)

type UnsubscribeUsecase struct {
	port subscription_port.SubscriptionPort
}

func NewUnsubscribeUsecase(port subscription_port.SubscriptionPort) *UnsubscribeUsecase {
	return &UnsubscribeUsecase{port: port}
}

func (u *UnsubscribeUsecase) Execute(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	return u.port.Unsubscribe(ctx, userID, feedLinkID)
}
