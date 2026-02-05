package feed_link_domain_port

import (
	"alt/domain"
	"context"
)

//go:generate go run go.uber.org/mock/mockgen -source=feed_link_domain_port.go -destination=../../mocks/mock_feed_link_domain_port.go -package=mocks FeedLinkDomainPort

// FeedLinkDomainPort defines the interface for extracting unique domains from feed_links
type FeedLinkDomainPort interface {
	// ListFeedLinkDomains extracts unique domains from feed_links table
	ListFeedLinkDomains(ctx context.Context) ([]domain.FeedLinkDomain, error)
}
