package today_digest_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Gateway implements today digest port interfaces using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new today digest gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// GetTodayDigest implements today_digest_port.GetTodayDigestPort.
func (g *Gateway) GetTodayDigest(ctx context.Context, userID uuid.UUID, date time.Time) (domain.TodayDigest, error) {
	if g.repo == nil {
		return domain.TodayDigest{}, fmt.Errorf("GetTodayDigest: database connection not available")
	}
	return g.repo.GetTodayDigest(ctx, userID, date)
}

// UpsertTodayDigest implements today_digest_port.UpsertTodayDigestPort.
func (g *Gateway) UpsertTodayDigest(ctx context.Context, digest domain.TodayDigest) error {
	if g.repo == nil {
		return fmt.Errorf("UpsertTodayDigest: database connection not available")
	}
	return g.repo.UpsertTodayDigest(ctx, digest)
}
