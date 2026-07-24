package di

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLogImageProxyWiringState pins CLAUDE.md rule 8 / di-wiring.md for the
// image proxy: cfg.ImageProxy.Enabled=true with an empty Secret must be
// loudly distinguishable from a deliberate IMAGE_PROXY_ENABLED=false, and
// must fail fast in production instead of silently leaving
// ImageProxyUsecaseInstance nil (which today no-ops every warmer/backfill
// job with only an Info-level "not configured" log — finding [6] / [12]).
func TestLogImageProxyWiringState(t *testing.T) {
	t.Run("enabled when Enabled=true and secret present", func(t *testing.T) {
		assert.True(t, logImageProxyWiringState(true, true, "development"))
	})

	t.Run("disabled but non-fatal when Enabled=false outside production", func(t *testing.T) {
		assert.False(t, logImageProxyWiringState(false, false, "development"))
		assert.False(t, logImageProxyWiringState(false, false, ""))
	})

	t.Run("misconfigured (enabled, no secret) is non-fatal outside production", func(t *testing.T) {
		assert.False(t, logImageProxyWiringState(true, false, "development"))
	})

	t.Run("panics when Enabled=true but secret empty in production", func(t *testing.T) {
		assert.Panics(t, func() {
			logImageProxyWiringState(true, false, "production")
		})
	})

	t.Run("does not panic when deliberately disabled in production", func(t *testing.T) {
		assert.NotPanics(t, func() {
			assert.False(t, logImageProxyWiringState(false, false, "production"))
		})
	})
}
