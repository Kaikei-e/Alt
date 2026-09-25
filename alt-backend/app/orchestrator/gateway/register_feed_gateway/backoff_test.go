package register_feed_gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalculateBackoff(t *testing.T) {
	initial := 100 * time.Millisecond
	max := 500 * time.Millisecond

	require.Equal(t, 100*time.Millisecond, calculateBackoff(1, initial, max))
	require.Equal(t, 200*time.Millisecond, calculateBackoff(2, initial, max))
	require.Equal(t, 400*time.Millisecond, calculateBackoff(3, initial, max))
	require.Equal(t, 500*time.Millisecond, calculateBackoff(4, initial, max))
	require.Equal(t, 500*time.Millisecond, calculateBackoff(10, initial, max))
}
