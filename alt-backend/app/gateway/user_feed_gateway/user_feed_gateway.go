package user_feed_gateway

import (
	"alt/driver/alt_db"
	"alt/port/user_feed_port"
	"context"

	"github.com/google/uuid"
)

// Gateway adapts the user feed port to the AltDBRepository.
type Gateway struct {
	altDBRepo *alt_db.AltDBRepository
}

// NewGateway constructs a user feed gateway.
func NewGateway(altDBRepo *alt_db.AltDBRepository) user_feed_port.UserFeedPort {
	return &Gateway{
		altDBRepo: altDBRepo,
	}
}

// GetUserFeedIDs returns the feed IDs that the user is subscribed to.
func (g *Gateway) GetUserFeedIDs(ctx context.Context) ([]uuid.UUID, error) {
	return g.altDBRepo.FetchUserFeedIDs(ctx)
}
