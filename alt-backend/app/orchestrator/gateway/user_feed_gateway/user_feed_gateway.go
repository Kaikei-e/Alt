package user_feed_gateway

import (
	"context"

	"alt/orchestrator/port/user_feed_port"
	"alt/shared/gateway/datahub_gateway"

	"github.com/google/uuid"
)

// userFeedClient is the slice of the alt-data-hub stats gateway this package
// adapts (capability catalog §2.M W3-M7).
type userFeedClient interface {
	UserFeedIDs(ctx context.Context) ([]uuid.UUID, error)
}

var _ userFeedClient = (*datahub_gateway.StatsGateway)(nil)

// Gateway adapts the user feed port to alt-data-hub.
type Gateway struct {
	stats userFeedClient
}

// NewGateway constructs a user feed gateway.
func NewGateway(stats userFeedClient) user_feed_port.UserFeedPort {
	return &Gateway{stats: stats}
}

// GetUserFeedIDs returns the feed IDs the signed-in user has read state
// against. The tenant is still resolved from the context — one layer down, in
// the gateway that turns it into a request field.
func (g *Gateway) GetUserFeedIDs(ctx context.Context) ([]uuid.UUID, error) {
	return g.stats.UserFeedIDs(ctx)
}
