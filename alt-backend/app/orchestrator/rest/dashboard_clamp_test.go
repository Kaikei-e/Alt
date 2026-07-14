package rest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// L-002: dashboard handlers must clamp the `limit` and `window` query
// parameters so that an admin (or compromised admin token) cannot DoS the
// backend with `limit=999999999&window=2147483647`.

func TestClampLimit_RespectsCeiling(t *testing.T) {
	require.Equal(t, int64(100), clampLimit(50000, 100, 200))
	require.Equal(t, int64(200), clampLimit(0, 100, 200), "zero falls back to default")
	require.Equal(t, int64(50), clampLimit(50, 100, 200), "in-range value passes through")
}

func TestClampWindowSeconds_RespectsCeiling(t *testing.T) {
	const day = int64(24 * 60 * 60)
	require.Equal(t, day, clampWindowSeconds(99999999, 14400, day))
	require.Equal(t, int64(14400), clampWindowSeconds(0, 14400, day))
	require.Equal(t, int64(3600), clampWindowSeconds(3600, 14400, day))
}

// Negative or sub-zero values must collapse to the default, never become
// large numbers via integer overflow tricks.
func TestClampLimit_NegativeFallsBackToDefault(t *testing.T) {
	require.Equal(t, int64(200), clampLimit(-1, 100, 200))
}
