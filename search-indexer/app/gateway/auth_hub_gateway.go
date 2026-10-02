package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"search-indexer/driver"
	"search-indexer/port"
)

type authHubGateway struct {
	driver driver.AuthHubDriver
}

// NewAuthHubGateway creates an AuthHub gateway adapting driver.AuthHubDriver to port.AuthHub.
func NewAuthHubGateway(d driver.AuthHubDriver) port.AuthHub {
	return &authHubGateway{
		driver: d,
	}
}

// IntrospectToken validates token via driver and enforces strict UUID and expiration invariants.
func (g *authHubGateway) IntrospectToken(ctx context.Context, token string) (*port.TokenIntrospection, error) {
	resp, err := g.driver.IntrospectToken(ctx, token)
	if err != nil {
		return nil, err
	}

	if resp.Active {
		// Strict UUID validation on sub
		if _, err := uuid.Parse(resp.Sub); err != nil {
			return nil, fmt.Errorf("invalid sub uuid in active token: %w", err)
		}
		// Strict expiration validation: active token must not be expired
		if resp.Exp <= time.Now().Unix() {
			return nil, fmt.Errorf("token expired at %d", resp.Exp)
		}
	}

	return &port.TokenIntrospection{
		Active:   resp.Active,
		Sub:      resp.Sub,
		TenantID: resp.TenantID,
		Exp:      resp.Exp,
	}, nil
}
