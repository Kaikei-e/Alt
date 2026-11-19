package subscription_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/subscription_port"
	"context"
	"errors"

	"github.com/google/uuid"
)

// SetSubscriptionDailyCapUsecase sets or clears the daily unread cap on one
// subscription of the calling user.
type SetSubscriptionDailyCapUsecase struct {
	port subscription_port.SubscriptionPort
}

func NewSetSubscriptionDailyCapUsecase(port subscription_port.SubscriptionPort) *SetSubscriptionDailyCapUsecase {
	if port == nil {
		panic("subscription_usecase: SubscriptionPort is required (see .claude/rules/di-wiring.md)")
	}
	return &SetSubscriptionDailyCapUsecase{port: port}
}

func (u *SetSubscriptionDailyCapUsecase) Execute(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID, dailyCap *int) error {
	if userID == uuid.Nil {
		return errors.New("user_id is required")
	}
	if feedLinkID == uuid.Nil {
		return errors.New("feed_link_id is required")
	}
	if ctxUser, err := domain.GetUserFromContext(ctx); err == nil && ctxUser.UserID != userID {
		return errors.New("cannot set daily cap for another user")
	}
	if dailyCap != nil && *dailyCap <= 0 {
		return errors.New("daily_cap must be greater than 0")
	}
	return u.port.SetSubscriptionDailyCap(ctx, userID, feedLinkID, dailyCap)
}
