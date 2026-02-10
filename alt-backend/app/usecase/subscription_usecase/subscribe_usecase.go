package subscription_usecase

import (
	"alt/port/subscription_port"
	"context"

	"github.com/google/uuid"
)

type SubscribeUsecase struct {
	port subscription_port.SubscriptionPort
}

func NewSubscribeUsecase(port subscription_port.SubscriptionPort) *SubscribeUsecase {
	return &SubscribeUsecase{port: port}
}

func (u *SubscribeUsecase) Execute(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	return u.port.Subscribe(ctx, userID, feedLinkID)
}
