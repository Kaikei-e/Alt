package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"

	"auth-hub/internal/domain"
)

// HMACCSRFGenerator generates CSRF tokens using HMAC-SHA256.
// Implements domain.CSRFTokenGenerator.
type HMACCSRFGenerator struct {
	secret []byte
}

// NewHMACCSRFGenerator creates a new CSRF token generator.
func NewHMACCSRFGenerator(secret string) *HMACCSRFGenerator {
	return &HMACCSRFGenerator{secret: []byte(secret)}
}

// Generate creates a deterministic CSRF token from a session ID.
func (g *HMACCSRFGenerator) Generate(sessionID string) (string, error) {
	if len(g.secret) == 0 {
		return "", domain.ErrCSRFSecretMissing
	}

	mac := hmac.New(sha256.New, g.secret)
	mac.Write([]byte(sessionID))
	return base64.URLEncoding.EncodeToString(mac.Sum(nil)), nil
}
