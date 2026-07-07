package di

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLogSovereignWiringState pins CLAUDE.md rule 8 / di-wiring.md: a missing
// SOVEREIGN_URL must be loudly distinguishable from a deliberately disabled
// client, and must fail fast in production instead of starting in limp mode
// with every Knowledge Home mutation silently no-op'ing.
func TestLogSovereignWiringState(t *testing.T) {
	t.Run("enabled when SOVEREIGN_URL is set", func(t *testing.T) {
		assert.True(t, logSovereignWiringState("http://knowledge-sovereign:9500", "development"))
	})

	t.Run("disabled but non-fatal outside production", func(t *testing.T) {
		assert.False(t, logSovereignWiringState("", "development"))
		assert.False(t, logSovereignWiringState("", ""))
	})

	t.Run("panics when unset in production", func(t *testing.T) {
		assert.Panics(t, func() {
			logSovereignWiringState("", "production")
		})
	})
}
