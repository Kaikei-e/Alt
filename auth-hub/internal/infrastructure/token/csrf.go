package token

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"auth-hub/internal/domain"
)

// DefaultCSRFTTL is how long a generated CSRF token remains valid.
const DefaultCSRFTTL = time.Hour

// HMACCSRFGenerator generates CSRF tokens using HMAC-SHA256.
// Implements domain.CSRFTokenGenerator.
//
// Tokens embed a unix-second timestamp so they rotate over time and can be
// rejected after TTL (replay resistance). Format: "<unix>.<b64url(mac)>" where
// mac = HMAC(secret, sessionID + "|" + unix).
type HMACCSRFGenerator struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewHMACCSRFGenerator creates a new CSRF token generator.
func NewHMACCSRFGenerator(secret string) *HMACCSRFGenerator {
	return &HMACCSRFGenerator{
		secret: []byte(secret),
		ttl:    DefaultCSRFTTL,
		now:    time.Now,
	}
}

// Generate creates a time-bound CSRF token from a session ID.
func (g *HMACCSRFGenerator) Generate(sessionID string) (string, error) {
	if len(g.secret) == 0 {
		return "", domain.ErrCSRFSecretMissing
	}

	ts := g.now().Unix()
	mac := g.sign(sessionID, ts)
	return fmt.Sprintf("%d.%s", ts, base64.URLEncoding.EncodeToString(mac)), nil
}

// Validate checks that token was issued for sessionID and is within TTL.
func (g *HMACCSRFGenerator) Validate(sessionID, token string) error {
	if len(g.secret) == 0 {
		return domain.ErrCSRFSecretMissing
	}
	if sessionID == "" || token == "" {
		return domain.ErrCSRFTokenInvalid
	}

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return domain.ErrCSRFTokenInvalid
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return domain.ErrCSRFTokenInvalid
	}
	mac, err := base64.URLEncoding.DecodeString(parts[1])
	if err != nil {
		return domain.ErrCSRFTokenInvalid
	}

	expected := g.sign(sessionID, ts)
	if subtle.ConstantTimeCompare(mac, expected) != 1 {
		return domain.ErrCSRFTokenInvalid
	}

	age := g.now().Sub(time.Unix(ts, 0))
	if age < 0 || age > g.ttl {
		return domain.ErrCSRFTokenExpired
	}
	return nil
}

func (g *HMACCSRFGenerator) sign(sessionID string, ts int64) []byte {
	mac := hmac.New(sha256.New, g.secret)
	mac.Write([]byte(sessionID))
	mac.Write([]byte("|"))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	return mac.Sum(nil)
}
