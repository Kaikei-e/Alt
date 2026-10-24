package authgw

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/authhub"
)

type fakeAuthHubDriver struct {
	claims *authhub.AuthHubClaims
	err    error
}

func (f *fakeAuthHubDriver) ValidateToken(_ context.Context, _ string) (*authhub.AuthHubClaims, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

func TestGateway_ValidateToken(t *testing.T) {
	ctx := context.Background()
	sub := uuid.New().String()
	tenantID := uuid.New().String()
	exp := time.Now().Add(5 * time.Minute).Unix()

	drv := &fakeAuthHubDriver{
		claims: &authhub.AuthHubClaims{
			Sub:      sub,
			TenantID: tenantID,
			Exp:      exp,
		},
	}
	gw := NewGateway(drv)
	assert.Equal(t, drv, gw.Driver())

	verified, err := gw.ValidateToken(ctx, "valid-token")
	require.NoError(t, err)
	require.NotNil(t, verified)
	assert.Equal(t, sub, verified.Subject)
	assert.Equal(t, tenantID, verified.TenantID)
	assert.Equal(t, time.Unix(exp, 0), verified.ExpiresAt)

	drv.err = errors.New("upstream auth failed")
	_, err = gw.ValidateToken(ctx, "bad-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream auth failed")
}
