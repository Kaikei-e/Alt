package di

import (
	"strings"
	"testing"
	"time"

	"alt/utils/rate_limiter"

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
			coordinator := NewHostRateLimiterCoordinator(tt.binary, tt.redisURL)
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
		NewHostRateLimiterCoordinator("alt-backend", "://not-a-url")
	})
}

// The external-API class (5s+) and the image-proxy class (1s) run at different
// intervals against hosts they can share (a CDN serves both the feed and its
// images). Coordinating them on one key would either throttle the images or
// break the feed promise, so the namespaces must differ.
func TestHostRateLimiterNamespacesAreDistinct(t *testing.T) {
	assert.NotEqual(t, rate_limiter.NamespaceExternalAPI, rate_limiter.NamespaceImageProxy)
}
