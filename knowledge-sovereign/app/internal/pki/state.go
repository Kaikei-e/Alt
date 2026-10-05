package pki

import "time"

// State classifies a leaf against the 2/3-lifetime renewal policy.
type State int

const (
	StateMissing State = iota
	StateFresh
	StateNearExpiry
	StateExpired
	StateCorrupt
)

func (s State) String() string {
	switch s {
	case StateMissing:
		return "missing"
	case StateFresh:
		return "fresh"
	case StateNearExpiry:
		return "near_expiry"
	case StateExpired:
		return "expired"
	case StateCorrupt:
		return "corrupt"
	default:
		return "unknown"
	}
}

// ClassifyRemaining is a pure function: identical inputs produce identical
// outputs. renewAtFraction 0.66 means "renew when 66% of the window elapsed".
func ClassifyRemaining(notBefore, notAfter, now time.Time, renewAtFraction float64) State {
	if now.After(notAfter) || now.Equal(notAfter) {
		return StateExpired
	}
	total := notAfter.Sub(notBefore)
	if total <= 0 {
		return StateExpired
	}
	elapsed := now.Sub(notBefore)
	if float64(elapsed)/float64(total) >= renewAtFraction {
		return StateNearExpiry
	}
	return StateFresh
}
