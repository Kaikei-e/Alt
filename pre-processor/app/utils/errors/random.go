package errors

import (
	crand "crypto/rand"
	"math"
	"math/big"
)

// SecureRandomFloat64 returns a cryptographically secure random number in [0,1).
// If the system random number generator fails, 0 is returned.
func SecureRandomFloat64() float64 {
	n, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return 0
	}
	return float64(n.Int64()) / float64(math.MaxInt64)
}
