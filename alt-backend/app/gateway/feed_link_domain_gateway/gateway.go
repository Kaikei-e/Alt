package feed_link_domain_gateway

import (
	"alt/domain"
	"alt/port/feed_link_domain_port"
	"context"
)

// FeedLinkDomainDriver defines the interface for the driver operations
type FeedLinkDomainDriver interface {
	ListFeedLinkDomains(ctx context.Context) ([]domain.FeedLinkDomain, error)
}

// FeedLinkDomainGateway implements the FeedLinkDomainPort interface
type FeedLinkDomainGateway struct {
	driver FeedLinkDomainDriver
}

// NewFeedLinkDomainGateway creates a new gateway instance
func NewFeedLinkDomainGateway(driver FeedLinkDomainDriver) feed_link_domain_port.FeedLinkDomainPort {
	return &FeedLinkDomainGateway{
		driver: driver,
	}
}

// ListFeedLinkDomains extracts unique domains from feed_links table
func (g *FeedLinkDomainGateway) ListFeedLinkDomains(ctx context.Context) ([]domain.FeedLinkDomain, error) {
	return g.driver.ListFeedLinkDomains(ctx)
}
