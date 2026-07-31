package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HOST_RATE_LIMITER_REDIS_URL is what turns the per-host interval from a
// per-process promise into a per-deployment one (ADR-000954 review, weakness
// 5). Rule 9 applies to it in both directions: a malformed value must stop
// startup, and an absent value must mean the explicit "local" mode rather than
// an inferred one.
func TestRateLimitConfig_CoordinationRedisURL(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    string
		wantErr bool
	}{
		{
			name: "unset is local mode",
			env:  "",
			want: "",
		},
		{
			name: "redis url is carried through",
			env:  "redis://redis-streams:6379/3",
			want: "redis://redis-streams:6379/3",
		},
		{
			name: "tls redis url is accepted",
			env:  "rediss://redis-streams:6379/3",
			want: "rediss://redis-streams:6379/3",
		},
		{
			name:    "a host:port with no scheme is rejected at startup",
			env:     "redis-streams:6379",
			wantErr: true,
		},
		{
			name:    "a postgres url in the redis slot is rejected at startup",
			env:     "postgres://user:pw@db:5432/alt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("HOST_RATE_LIMITER_REDIS_URL", tt.env)
			}

			cfg := &Config{}
			require.NoError(t, loadFromEnvironment(cfg))

			err := validateRateLimitConfig(&cfg.RateLimit)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "HOST_RATE_LIMITER_REDIS_URL")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.RateLimit.CoordinationRedisURL)
		})
	}
}
