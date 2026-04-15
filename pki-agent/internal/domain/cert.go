// Package domain defines the core entities and sentinel errors for pki-agent.
// It has zero dependencies on the rest of the stack.
package domain

import "time"

// CertState classifies a certificate on disk against the rotation policy.
type CertState int

const (
	// StateMissing: no cert file on disk.
	StateMissing CertState = iota
	// StateFresh: cert valid and far from expiry.
	StateFresh
	// StateNearExpiry: past the renewal threshold, not yet expired.
	StateNearExpiry
	// StateExpired: not_after < now.
	StateExpired
	// StateCorrupt: file exists but cannot be parsed.
	StateCorrupt
)

func (s CertState) String() string {
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

// ClassifyRemaining returns the state for a cert whose validity window is
// [notBefore, notAfter], inspected at now, given renewAtFraction of the total
// lifetime (e.g. 0.66 -> renew when 66% of the window has elapsed).
//
// Pure function: no side effects, identical inputs produce identical outputs.
func ClassifyRemaining(notBefore, notAfter, now time.Time, renewAtFraction float64) CertState {
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
