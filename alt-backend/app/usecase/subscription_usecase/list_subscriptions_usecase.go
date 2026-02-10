package subscription_usecase

import (
	"alt/domain"
	"alt/port/subscription_port"
	"context"

	"github.com/google/uuid"
)

type ListSubscriptionsUsecase struct {
	port subscription_port.SubscriptionPort
}

func NewListSubscriptionsUsecase(port subscription_port.SubscriptionPort) *ListSubscriptionsUsecase {
	return &ListSubscriptionsUsecase{port: port}
}

func (u *ListSubscriptionsUsecase) Execute(ctx context.Context, userID uuid.UUID) ([]*domain.FeedSource, error) {
	return u.port.ListSubscriptions(ctx, userID)
}
