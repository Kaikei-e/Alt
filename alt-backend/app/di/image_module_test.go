package di

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLogImageProxyWiringState pins CLAUDE.md rule 8 / di-wiring.md for the
// image proxy: cfg.ImageProxy.Enabled=true with an empty Secret must be
// loudly distinguishable from a deliberate IMAGE_PROXY_ENABLED=false, and
// must fail fast instead of silently leaving ImageProxyUsecaseInstance nil
// (which today no-ops every warmer/backfill job with only an Info-level
// "not configured" log — finding [6] / [12]). The panic is no longer keyed
// on APP_ENV=production: APP_ENV is set in no compose file, so that made the
// guard unreachable.
func TestLogImageProxyWiringState(t *testing.T) {
	t.Run("enabled when Enabled=true and secret present", func(t *testing.T) {
		assert.True(t, logImageProxyWiringState(true, true))
	})

	t.Run("disabled but non-fatal when Enabled=false", func(t *testing.T) {
		assert.False(t, logImageProxyWiringState(false, false))
	})

	t.Run("panics when Enabled=true but secret empty", func(t *testing.T) {
		assert.Panics(t, func() {
			logImageProxyWiringState(true, false)
		})
	})

	t.Run("does not panic when deliberately disabled", func(t *testing.T) {
		assert.NotPanics(t, func() {
			assert.False(t, logImageProxyWiringState(false, false))
		})
	})
}
