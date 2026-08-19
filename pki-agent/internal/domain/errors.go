package domain

import "errors"

// Sentinel errors for pki-agent. Prefer errors.Is() against these in tests and
// handlers; never match on strings.
var (
	// ErrCertNotFound: cert file absent from disk.
	ErrCertNotFound = errors.New("pki-agent: cert not found")
	// ErrCertParseFailed: file present but not a valid X.509 PEM.
	ErrCertParseFailed = errors.New("pki-agent: cert parse failed")
	// ErrCAUnreachable: step-ca HTTP call failed or timed out.
	ErrCAUnreachable = errors.New("pki-agent: CA unreachable")
	// ErrCARejected: step-ca returned a non-2xx (including 401 / 400).
	ErrCARejected = errors.New("pki-agent: CA rejected request")
	// ErrTokenSign: failed to sign the one-time token (JWK / JWE).
	ErrTokenSign = errors.New("pki-agent: OTT signing failed")
	// ErrCertChainInvalid: issued leaf did not verify against configured root.
	ErrCertChainInvalid = errors.New("pki-agent: leaf failed chain verification")
)

// LogSafeError returns a stable, non-secret identifier for ops logs.
// It never echoes PasswordFile paths or password bytes.
func LogSafeError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrCertNotFound):
		return "cert_not_found"
	case errors.Is(err, ErrCertParseFailed):
		return "cert_parse_failed"
	case errors.Is(err, ErrCAUnreachable):
		return "ca_unreachable"
	case errors.Is(err, ErrCARejected):
		return "ca_rejected"
	case errors.Is(err, ErrTokenSign):
		return "token_sign"
	case errors.Is(err, ErrCertChainInvalid):
		return "cert_chain_invalid"
	default:
		return "tick_error"
	}
}
