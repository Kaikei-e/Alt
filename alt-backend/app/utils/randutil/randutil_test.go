package randutil

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJitterInt64_StaysInInclusiveRange(t *testing.T) {
	const samples = 200
	const maxInclusive = int64(1000)

	for range samples {
		n, err := JitterInt64(maxInclusive)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, n, int64(0))
		assert.LessOrEqual(t, n, maxInclusive)
	}

	n, err := JitterInt64(0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestJitterInt64_RejectsNegativeBound(t *testing.T) {
	_, err := JitterInt64(-1)
	require.Error(t, err)
}

// maxInclusive+1 overflows int64 at MaxInt64; the bound must be computed in
// big.Int space instead of wrapping negative and panicking inside rand.Int.
func TestJitterInt64_MaxInt64BoundDoesNotOverflow(t *testing.T) {
	n, err := JitterInt64(math.MaxInt64)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(0))
}
