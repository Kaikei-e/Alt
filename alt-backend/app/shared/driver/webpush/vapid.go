package webpush

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// vapidTokenLifetime stays well inside the 24h ceiling of RFC 8292 section 2;
	// Apple rejects anything beyond a day outright.
	vapidTokenLifetime = 12 * time.Hour

	vapidPrivateKeyLength = 32
	vapidSignatureLength  = 64
)

// vapidJWTHeader is the encoded {"typ":"JWT","alg":"ES256"} header. It is fixed,
// so it is encoded once rather than marshalled per token.
var vapidJWTHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))

// vapidClaims is a struct rather than a map so the field order is stable.
type vapidClaims struct {
	Aud string `json:"aud"`
	Exp int64  `json:"exp"`
	Sub string `json:"sub"`
}

// VAPIDSigner issues RFC 8292 Authorization headers. It is safe for concurrent
// use, and caches one token per audience.
type VAPIDSigner struct {
	subject    string
	privateKey *ecdsa.PrivateKey
	publicKey  string

	nowFn func() time.Time

	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	header    string
	refreshAt time.Time
}

// NewVAPIDSigner builds a signer from the application server's raw P-256 private
// key, base64-encoded. The public key advertised in the k= parameter is derived
// from it, so a mismatched pair is impossible to configure.
func NewVAPIDSigner(subject, privateKeyB64 string) (*VAPIDSigner, error) {
	normalisedSubject, err := normaliseSubject(subject)
	if err != nil {
		return nil, err
	}

	rawKey, err := decodeKey("VAPID private", privateKeyB64, vapidPrivateKeyLength)
	if err != nil {
		return nil, err
	}
	privateKey, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), rawKey)
	if err != nil {
		return nil, fmt.Errorf("webpush parse VAPID private key: %w", err)
	}
	publicKey, err := privateKey.PublicKey.Bytes()
	if err != nil {
		return nil, fmt.Errorf("webpush encode VAPID public key: %w", err)
	}

	return &VAPIDSigner{
		subject:    normalisedSubject,
		privateKey: privateKey,
		publicKey:  base64.RawURLEncoding.EncodeToString(publicKey),
		nowFn:      time.Now,
		tokens:     make(map[string]cachedToken),
	}, nil
}

// Subject returns the normalised sub claim.
func (v *VAPIDSigner) Subject() string { return v.subject }

// PublicKey returns the base64url uncompressed application server key, which is
// also what the browser needs as applicationServerKey when subscribing.
func (v *VAPIDSigner) PublicKey() string { return v.publicKey }

// AuthorizationHeader returns the vapid Authorization header value for endpoint.
func (v *VAPIDSigner) AuthorizationHeader(endpoint string) (string, error) {
	audience, err := audienceOf(endpoint)
	if err != nil {
		return "", err
	}

	now := v.nowFn()

	// The lock spans signing so a burst of sends to one audience produces a
	// single signature rather than one per goroutine.
	v.mu.Lock()
	defer v.mu.Unlock()

	if token, ok := v.tokens[audience]; ok && now.Before(token.refreshAt) {
		return token.header, nil
	}

	header, err := v.sign(audience, now)
	if err != nil {
		return "", err
	}
	v.tokens[audience] = cachedToken{
		header: header,
		// Refresh at half life: Apple asks senders not to re-issue more than
		// once an hour, and this keeps a token valid for the whole retry window.
		refreshAt: now.Add(vapidTokenLifetime / 2),
	}
	return header, nil
}

func (v *VAPIDSigner) sign(audience string, now time.Time) (string, error) {
	claims, err := json.Marshal(vapidClaims{
		Aud: audience,
		Exp: now.Add(vapidTokenLifetime).Unix(),
		Sub: v.subject,
	})
	if err != nil {
		return "", fmt.Errorf("webpush marshal VAPID claims: %w", err)
	}

	signingInput := vapidJWTHeader + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))

	// JWS ES256 is a fixed-width R||S pair. SignASN1 would emit DER, which every
	// push service rejects with an unexplained 403.
	r, s, err := ecdsa.Sign(rand.Reader, v.privateKey, digest[:])
	if err != nil {
		return "", fmt.Errorf("webpush sign VAPID token: %w", err)
	}
	signature := make([]byte, vapidSignatureLength)
	r.FillBytes(signature[:vapidSignatureLength/2])
	s.FillBytes(signature[vapidSignatureLength/2:])

	jwt := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
	return "vapid t=" + jwt + ", k=" + v.publicKey, nil
}

// audienceOf reduces an endpoint to its scheme and host. Leaving the path in the
// aud claim is a common and badly reported cause of 403 from push services.
func audienceOf(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("webpush parse endpoint %q: %w", endpoint, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("webpush endpoint %q must be an absolute URL with a host", endpoint)
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// normaliseSubject accepts a bare email or an already-schemed mailto:/https:
// value and yields exactly one scheme. Prefixing unconditionally is what produces
// "mailto:mailto:...", which Apple rejects with 403 BadJwtToken while other push
// services quietly tolerate it.
func normaliseSubject(subject string) (string, error) {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return "", fmt.Errorf("webpush VAPID subject is required")
	}

	if rest, ok := trimPrefixFold(trimmed, "mailto:"); ok {
		// Loop so an already-doubled subject collapses rather than compounding.
		for {
			next, ok := trimPrefixFold(strings.TrimSpace(rest), "mailto:")
			if !ok {
				break
			}
			rest = next
		}
		address := strings.TrimSpace(rest)
		if !isEmailLike(address) {
			return "", fmt.Errorf("webpush VAPID subject %q is not a valid mailto address", subject)
		}
		return "mailto:" + address, nil
	}

	if _, ok := trimPrefixFold(trimmed, "https://"); ok {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", fmt.Errorf("webpush parse VAPID subject %q: %w", subject, err)
		}
		if parsed.Host == "" {
			return "", fmt.Errorf("webpush VAPID subject %q must have a host", subject)
		}
		return trimmed, nil
	}

	if isEmailLike(trimmed) {
		return "mailto:" + trimmed, nil
	}

	return "", fmt.Errorf("webpush VAPID subject %q must be an email address, a mailto: URI, or an https: URL", subject)
}

func trimPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return s, false
	}
	return s[len(prefix):], true
}

// isEmailLike is deliberately loose: this is operator-supplied config, and strict
// email validation rejects more valid addresses than it catches typos.
func isEmailLike(s string) bool {
	local, domain, ok := strings.Cut(s, "@")
	return ok && local != "" && domain != "" &&
		!strings.ContainsAny(s, " \t\r\n") && strings.Contains(domain, ".")
}
