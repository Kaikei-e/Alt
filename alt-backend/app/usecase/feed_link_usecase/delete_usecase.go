package feed_link_usecase

import (
	"alt/port/feed_link_port"
	"context"

	"github.com/google/uuid"
)

type DeleteFeedLinkUsecase struct {
	port feed_link_port.FeedLinkPort
}

func NewDeleteFeedLinkUsecase(port feed_link_port.FeedLinkPort) *DeleteFeedLinkUsecase {
	return &DeleteFeedLinkUsecase{port: port}
}

func (u *DeleteFeedLinkUsecase) Execute(ctx context.Context, id uuid.UUID) error {
	return u.port.DeleteFeedLink(ctx, id)
}
