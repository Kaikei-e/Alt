package register_feed_usecase

import (
	"alt/port/register_feed_port"
	"context"
)

type RegisterFeedUsecase struct {
	registerFeedGateway register_feed_port.RegisterFeedPort
}

func NewRegisterFeedUsecase(registerFeedGateway register_feed_port.RegisterFeedPort) *RegisterFeedUsecase {
	return &RegisterFeedUsecase{
		registerFeedGateway: registerFeedGateway,
	}
}

func (r *RegisterFeedUsecase) Execute(ctx context.Context, link string) error {
	return r.registerFeedGateway.RegisterRSSFeedLink(ctx, link)
}
