package feed_link_usecase

import (
	"alt/domain"
	"alt/port/feed_link_port"
	"context"
)

type ListFeedLinksUsecase struct {
	port feed_link_port.FeedLinkPort
}

func NewListFeedLinksUsecase(port feed_link_port.FeedLinkPort) *ListFeedLinksUsecase {
	return &ListFeedLinksUsecase{port: port}
}

func (u *ListFeedLinksUsecase) Execute(ctx context.Context) ([]*domain.FeedLink, error) {
	return u.port.ListFeedLinks(ctx)
}
