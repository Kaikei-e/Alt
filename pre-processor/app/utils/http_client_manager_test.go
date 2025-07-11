package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPClientManager_GetDefaultClient(t *testing.T) {
	tests := []struct {
		name string
		want struct {
			timeout             time.Duration
			maxIdleConns        int
			maxIdleConnsPerHost int
			idleConnTimeout     time.Duration
			tlsHandshakeTimeout time.Duration
		}
	}{
		{
			name: "should return optimized default client",
			want: struct {
				timeout             time.Duration
				maxIdleConns        int
				maxIdleConnsPerHost int
				idleConnTimeout     time.Duration
				tlsHandshakeTimeout time.Duration
			}{
				timeout:             30 * time.Second,
				maxIdleConns:        100,
				maxIdleConnsPerHost: 10,
				idleConnTimeout:     90 * time.Second,
				tlsHandshakeTimeout: 10 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewHTTPClientManager()
			client := manager.GetDefaultClient()

			require.NotNil(t, client)
			assert.Equal(t, tt.want.timeout, client.Timeout)

			// Check transport settings
			transport := client.Transport.(*optimizedTransport)
			assert.Equal(t, tt.want.maxIdleConns, transport.MaxIdleConns)
			assert.Equal(t, tt.want.maxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
			assert.Equal(t, tt.want.idleConnTimeout, transport.IdleConnTimeout)
			assert.Equal(t, tt.want.tlsHandshakeTimeout, transport.TLSHandshakeTimeout)
		})
	}
}

func TestHTTPClientManager_GetSummaryClient(t *testing.T) {
	tests := []struct {
		name string
		want struct {
			timeout time.Duration
		}
	}{
		{
			name: "should return client with longer timeout for summary API",
			want: struct {
				timeout time.Duration
			}{
				timeout: 60 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewHTTPClientManager()
			client := manager.GetSummaryClient()

			require.NotNil(t, client)
			assert.Equal(t, tt.want.timeout, client.Timeout)
		})
	}
}

func TestHTTPClientManager_GetFeedClient(t *testing.T) {
	tests := []struct {
		name string
		want struct {
			timeout time.Duration
		}
	}{
		{
			name: "should return client with fast timeout for feed fetching",
			want: struct {
				timeout time.Duration
			}{
				timeout: 15 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewHTTPClientManager()
			client := manager.GetFeedClient()

			require.NotNil(t, client)
			assert.Equal(t, tt.want.timeout, client.Timeout)
		})
	}
}

func TestHTTPClientManager_Singleton(t *testing.T) {
	t.Run("should return same instance for multiple calls", func(t *testing.T) {
		manager1 := NewHTTPClientManager()
		manager2 := NewHTTPClientManager()

		client1 := manager1.GetDefaultClient()
		client2 := manager2.GetDefaultClient()

		// Should be the same client instance (singleton behavior)
		assert.Same(t, client1, client2)
	})
}
