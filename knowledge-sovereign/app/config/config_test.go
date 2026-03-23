package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoad_DefaultAddresses(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("METRICS_ADDR", "")
	t.Setenv("SERVICE_SECRET", "")
	t.Setenv("SERVICE_SECRET_FILE", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, ":9500", cfg.ListenAddr)
	assert.Equal(t, ":9501", cfg.MetricsAddr)
}
