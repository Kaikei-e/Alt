// Package randutil holds the crypto/rand-backed jitter helper shared by the
// retry/backoff paths (push dispatch, distributed host rate limiting).
package randutil

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// JitterInt64 returns a uniformly random value in [0, maxInclusive].
func JitterInt64(maxInclusive int64) (int64, error) {
	if maxInclusive < 0 {
		return 0, fmt.Errorf("negative jitter bound")
	}
	// The bound is exclusive and computed in big.Int space: maxInclusive+1
	// would wrap at MaxInt64 and panic inside rand.Int.
	bound := new(big.Int).Add(big.NewInt(maxInclusive), big.NewInt(1))
	n, err := rand.Int(rand.Reader, bound)
	if err != nil {
		return 0, fmt.Errorf("crypto/rand jitter: %w", err)
	}
	return n.Int64(), nil
}
