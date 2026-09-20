package di

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"alt/utils/rate_limiter"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHostRateLimiterCoordinator(t *testing.T) {
	tests := []struct {
		name     string
		binary   string
		redisURL string
		wantMode string
	}{
		{
			name:     "a redis url puts every limiter in distributed mode",
			binary:   "alt-backend",
			redisURL: "redis://redis-streams:6379/3",
			wantMode: rate_limiter.ModeDistributed,
		},
		{
			name:     "an unset url is the explicit local mode",
			binary:   "alt-harvester",
			redisURL: "",
			wantMode: rate_limiter.ModeLocal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coordinator := NewHostRateLimiterCoordinator(tt.binary, tt.redisURL, "")
			require.NotNil(t, coordinator)
			assert.Equal(t, tt.wantMode, coordinator.Mode())

			limiter := coordinator.Limiter(rate_limiter.NamespaceExternalAPI, 5*time.Second, 1)
			require.NotNil(t, limiter)
			assert.Equal(t, tt.wantMode, limiter.Mode(),
				"the mode the composition root logged must be the mode the limiter actually runs in")

			assert.True(t, strings.HasPrefix(coordinator.Owner(), tt.binary+"/"),
				"the slot owner names the binary, so GET on a contended key says who is holding the host")
		})
	}
}

// A URL that config validation would have rejected can only get here if the
// composition root skipped the check. Limping on with the local guarantee
// while the operator believes coordination is on is exactly the silent
// degradation rule 8 forbids.
func TestNewHostRateLimiterCoordinator_PanicsOnUnusableURL(t *testing.T) {
	assert.Panics(t, func() {
		NewHostRateLimiterCoordinator("alt-backend", "://not-a-url", "")
	})
}

// container_harvester.go / infra_module.go pass cfg.RateLimit.CoordinationRedisPassword
// as the now-required third argument (item 4/6). This proves it actually
// reaches the arbiter connection, not just that the parameter compiles: a
// password-protected arbiter answers the startup ping only when the caller
// authenticated correctly.
func TestNewHostRateLimiterCoordinator_PasswordReachesTheArbiter(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.RequireAuth("hunter2")

	t.Run("the correct password lets the startup ping succeed", func(t *testing.T) {
		var buf bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		coordinator := NewHostRateLimiterCoordinator("alt-backend", "redis://"+mr.Addr()+"/0", "hunter2")
		require.NotNil(t, coordinator)
		t.Cleanup(func() { _ = coordinator.Close() })

		assert.Equal(t, rate_limiter.ModeDistributed, coordinator.Mode())
		assert.NotContains(t, buf.String(), "arbiter_unreachable_at_startup",
			"the resolved password must let the startup ping authenticate")
	})

	t.Run("a missing password fails the startup ping against the same arbiter", func(t *testing.T) {
		var buf bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
		t.Cleanup(func() { slog.SetDefault(previous) })

		coordinator := NewHostRateLimiterCoordinator("alt-backend", "redis://"+mr.Addr()+"/0", "")
		require.NotNil(t, coordinator)
		t.Cleanup(func() { _ = coordinator.Close() })

		assert.Contains(t, buf.String(), "arbiter_unreachable_at_startup",
			"an unauthenticated connection against a password-protected arbiter must not look healthy")
	})
}

// The external-API class (5s+) and the image-proxy class (1s) run at different
// intervals against hosts they can share (a CDN serves both the feed and its
// images). Coordinating them on one key would either throttle the images or
// break the feed promise, so the namespaces must differ.
func TestHostRateLimiterNamespacesAreDistinct(t *testing.T) {
	assert.NotEqual(t, rate_limiter.NamespaceExternalAPI, rate_limiter.NamespaceImageProxy)
}
