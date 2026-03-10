package feed_link_usecase

import (
	"alt/domain"
	"alt/port/feed_link_port"
	"context"
)

type ListFeedLinksWithHealthUsecase struct {
	port feed_link_port.FeedLinkPort
}

func NewListFeedLinksWithHealthUsecase(port feed_link_port.FeedLinkPort) *ListFeedLinksWithHealthUsecase {
	return &ListFeedLinksWithHealthUsecase{port: port}
}

func (u *ListFeedLinksWithHealthUsecase) Execute(ctx context.Context) ([]*domain.FeedLinkWithHealth, error) {
	return u.port.ListFeedLinksWithHealth(ctx)
}
