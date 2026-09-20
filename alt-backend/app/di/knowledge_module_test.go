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
	t.Run("enabled when SOVEREIGN_URL is set with event auth enabled", func(t *testing.T) {
		assert.True(t, LogSovereignWiringState("alt-backend", "http://knowledge-sovereign:9500", "development", true))
	})

	t.Run("enabled when SOVEREIGN_URL is set with event auth disabled", func(t *testing.T) {
		assert.True(t, LogSovereignWiringState("alt-backend", "http://knowledge-sovereign:9500", "development", false))
	})

	t.Run("disabled but non-fatal outside production", func(t *testing.T) {
		assert.False(t, LogSovereignWiringState("alt-backend", "", "development", false))
		assert.False(t, LogSovereignWiringState("alt-backend", "", "", false))
	})

	t.Run("panics when unset in production", func(t *testing.T) {
		assert.Panics(t, func() {
			LogSovereignWiringState("alt-backend", "", "production", false)
		})
	})
}
