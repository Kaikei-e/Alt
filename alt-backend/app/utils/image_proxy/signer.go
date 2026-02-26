package image_proxy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// Signer handles HMAC-SHA256 signing for image proxy URLs.
type Signer struct {
	secret []byte
}

// NewSigner creates a new Signer with the given secret.
func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret)}
}

// GenerateProxyURL generates a signed proxy URL path for the given image URL.
// Returns the path: /v1/images/proxy/{hex-hmac}/{base64url-encoded-url}
func (s *Signer) GenerateProxyURL(imageURL string) string {
	if imageURL == "" {
		return ""
	}

	sig := s.sign(imageURL)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(imageURL))
	return fmt.Sprintf("/v1/images/proxy/%s/%s", sig, encoded)
}

// VerifyAndDecode verifies the HMAC signature and decodes the URL.
// Returns the original URL if valid, or an error if the signature is invalid.
func (s *Signer) VerifyAndDecode(signature, encodedURL string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encodedURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL encoding: %w", err)
	}

	originalURL := string(decoded)
	expectedSig := s.sign(originalURL)

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return "", fmt.Errorf("invalid signature")
	}

	return originalURL, nil
}

// sign generates an HMAC-SHA256 hex signature for the given URL.
func (s *Signer) sign(url string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(url))
	return hex.EncodeToString(mac.Sum(nil))
}

// URLHash returns the SHA-256 hex hash of a URL, used as cache key.
func URLHash(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:])
}
