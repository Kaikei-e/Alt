package consumer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConsumer_RedisPassword(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.RedisURL = "redis://localhost:6379"
	cfg.RedisPassword = "test-secret-password"

	c, err := NewConsumer(cfg, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, c.client.Close())
	})

	assert.Equal(t, "test-secret-password", c.client.Options().Password)
}
