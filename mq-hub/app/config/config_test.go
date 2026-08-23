package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfig_ReplyStreamSweepEnabled(t *testing.T) {
	t.Run("defaults to enabled when unset", func(t *testing.T) {
		t.Setenv("REPLY_STREAM_SWEEP_ENABLED", "")
		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.True(t, cfg.ReplyStreamSweepEnabled)
	})

	t.Run("can be explicitly disabled", func(t *testing.T) {
		t.Setenv("REPLY_STREAM_SWEEP_ENABLED", "false")
		cfg, err := NewConfig()
		require.NoError(t, err)
		assert.False(t, cfg.ReplyStreamSweepEnabled)
	})

	t.Run("fails fast on an unparseable value", func(t *testing.T) {
		t.Setenv("REPLY_STREAM_SWEEP_ENABLED", "sometimes")
		_, err := NewConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "REPLY_STREAM_SWEEP_ENABLED")
	})
}
