package di

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"alt/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func adminMonitorConfig(enabled bool, prometheusURL string) *config.Config {
	return &config.Config{
		AdminMonitor: config.AdminMonitorConfig{
			Enabled:        enabled,
			PrometheusURL:  prometheusURL,
			QueryTimeout:   3 * time.Second,
			CacheTTL:       10 * time.Second,
			RateLimitRPS:   5,
			RateLimitBurst: 10,
			StreamInterval: 5 * time.Second,
		},
	}
}

// TestNewAdminMonitorModule pins CLAUDE.md rule 8 / di-wiring.md for the admin
// observability pipeline. ADMIN_MONITOR_ENABLED=true is an operator decision,
// so a driver that cannot be built is a misconfiguration, not an opt-out:
// flipping Enabled back to false made connect/v2/server.go log
// "AdminMonitorService disabled (config.AdminMonitor.Enabled=false)" about a
// config value that says the opposite, and left every admin monitoring RPC
// answering 404 with no way to tell a bad ADMIN_MONITOR_PROMETHEUS_URL from an
// unset flag.
func TestNewAdminMonitorModule(t *testing.T) {
	t.Run("disabled flag leaves the module unwired without panicking", func(t *testing.T) {
		var m *AdminMonitorModule
		assert.NotPanics(t, func() {
			m = newAdminMonitorModule(adminMonitorConfig(false, ""), discardLogger())
		})
		require.NotNil(t, m)
		assert.False(t, m.Enabled)
		assert.Nil(t, m.Facade)
	})

	t.Run("enabled with a usable prometheus url wires the facade", func(t *testing.T) {
		m := newAdminMonitorModule(adminMonitorConfig(true, "http://prometheus:9090"), discardLogger())
		require.NotNil(t, m)
		assert.True(t, m.Enabled)
		assert.NotNil(t, m.Port)
		assert.NotNil(t, m.Facade)
	})

	t.Run("enabled but unbuildable driver panics instead of rewriting the flag", func(t *testing.T) {
		assert.Panics(t, func() {
			newAdminMonitorModule(adminMonitorConfig(true, ""), discardLogger())
		})
	})
}
