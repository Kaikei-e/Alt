package authgw

import (
	"context"
	"time"

	"knowledge-sovereign/domain/authcontext"
	"knowledge-sovereign/driver/authhub"
)

type AuthHubDriver interface {
	ValidateToken(ctx context.Context, tokenStr string) (*authhub.AuthHubClaims, error)
}

type Gateway struct {
	driver AuthHubDriver
}

func NewGateway(driver AuthHubDriver) *Gateway {
	return &Gateway{driver: driver}
}

func (g *Gateway) ValidateToken(ctx context.Context, tokenStr string) (*authcontext.VerifiedClaims, error) {
	claims, err := g.driver.ValidateToken(ctx, tokenStr)
	if err != nil {
		return nil, err
	}
	return &authcontext.VerifiedClaims{
		Subject:   claims.Sub,
		TenantID:  claims.TenantID,
		ExpiresAt: time.Unix(claims.Exp, 0),
	}, nil
}

func (g *Gateway) Driver() AuthHubDriver { return g.driver }
