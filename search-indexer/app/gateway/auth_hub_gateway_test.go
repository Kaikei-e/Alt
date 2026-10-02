package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"search-indexer/driver"
)

type mockAuthHubDriver struct {
	resp *driver.TokenIntrospectionResponse
	err  error
}

func (m *mockAuthHubDriver) IntrospectToken(ctx context.Context, token string) (*driver.TokenIntrospectionResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func TestAuthHubGateway_StrictUUIDValidation(t *testing.T) {
	mockD := &mockAuthHubDriver{
		resp: &driver.TokenIntrospectionResponse{
			Active: true,
			Sub:    "not-a-uuid",
			Exp:    time.Now().Add(1 * time.Hour).Unix(),
		},
	}
	gw := NewAuthHubGateway(mockD)

	_, err := gw.IntrospectToken(context.Background(), "token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid sub uuid")
}

func TestAuthHubGateway_StrictExpValidation(t *testing.T) {
	mockD := &mockAuthHubDriver{
		resp: &driver.TokenIntrospectionResponse{
			Active: true,
			Sub:    "00000000-0000-0000-0000-000000000001",
			Exp:    time.Now().Add(-1 * time.Minute).Unix(), // Expired
		},
	}
	gw := NewAuthHubGateway(mockD)

	_, err := gw.IntrospectToken(context.Background(), "token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token expired")
}

func TestAuthHubGateway_Success(t *testing.T) {
	mockD := &mockAuthHubDriver{
		resp: &driver.TokenIntrospectionResponse{
			Active:   true,
			Sub:      "00000000-0000-0000-0000-000000000001",
			TenantID: "tenant-1",
			Exp:      time.Now().Add(1 * time.Hour).Unix(),
		},
	}
	gw := NewAuthHubGateway(mockD)

	info, err := gw.IntrospectToken(context.Background(), "token")
	require.NoError(t, err)
	assert.True(t, info.Active)
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", info.Sub)
	assert.Equal(t, "tenant-1", info.TenantID)
}

func TestAuthHubGateway_InactiveToken(t *testing.T) {
	mockD := &mockAuthHubDriver{
		resp: &driver.TokenIntrospectionResponse{
			Active: false,
		},
	}
	gw := NewAuthHubGateway(mockD)

	info, err := gw.IntrospectToken(context.Background(), "token")
	require.NoError(t, err)
	assert.False(t, info.Active)
}

func TestAuthHubGateway_DriverError(t *testing.T) {
	mockD := &mockAuthHubDriver{
		err: errors.New("network failure"),
	}
	gw := NewAuthHubGateway(mockD)

	_, err := gw.IntrospectToken(context.Background(), "token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network failure")
}
