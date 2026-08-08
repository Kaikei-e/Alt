// Package webpush sends encrypted Web Push messages using only the standard
// library: RFC 8291 message encryption, RFC 8292 VAPID authentication, and the
// RFC 8030 delivery protocol.
package webpush

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	saltLength       = 16
	publicKeyLength  = 65 // X9.62 uncompressed P-256 point
	authSecretLength = 16
	gcmTagLength     = 16
	paddingDelimiter = 0x02

	// headerLength is the RFC 8188 header: salt || rs || idlen || keyid.
	headerLength = saltLength + 4 + 1 + publicKeyLength

	// recordSize is advertised in the header. RFC 8291 section 4 mandates a
	// single record, so this is only ever an upper bound, never a split point.
	recordSize = 4096

	// MaxPayloadLength is the largest plaintext that still fits a 4096-octet
	// body once the header, the padding delimiter and the GCM tag are added.
	// Push services reject anything larger with 413, so this is checked before
	// the request is built rather than after it is rejected.
	MaxPayloadLength = recordSize - headerLength - gcmTagLength - 1
)

const (
	webPushInfo = "WebPush: info\x00"
	cekInfo     = "Content-Encoding: aes128gcm\x00"
	nonceInfo   = "Content-Encoding: nonce\x00"
)

// Subscription is a browser's PushSubscription as delivered by the Push API.
type Subscription struct {
	Endpoint string           `json:"endpoint"`
	Keys     SubscriptionKeys `json:"keys"`
}

// SubscriptionKeys carries the base64-encoded key material from a subscription.
type SubscriptionKeys struct {
	// P256dh is the user agent's uncompressed P-256 public key.
	P256dh string `json:"p256dh"`
	// Auth is the 16-octet authentication secret.
	Auth string `json:"auth"`
}

// Encrypter produces RFC 8291 aes128gcm message bodies. Construct it with
// NewEncrypter; the salt and ephemeral key sources are fields so the RFC test
// vectors can be reproduced exactly.
type Encrypter struct {
	saltFn      func() ([]byte, error)
	ephemeralFn func() (*ecdh.PrivateKey, error)
}

func NewEncrypter() *Encrypter {
	return &Encrypter{saltFn: randomSalt, ephemeralFn: generateEphemeralKey}
}

// Encrypt seals plaintext for the given subscription, returning the complete
// body to POST to the push endpoint.
func (e *Encrypter) Encrypt(sub Subscription, plaintext []byte) ([]byte, error) {
	uaPublic, err := decodeKey("p256dh", sub.Keys.P256dh, publicKeyLength)
	if err != nil {
		return nil, err
	}
	authSecret, err := decodeKey("auth", sub.Keys.Auth, authSecretLength)
	if err != nil {
		return nil, err
	}

	salt, err := e.saltFn()
	if err != nil {
		return nil, fmt.Errorf("webpush generate salt: %w", err)
	}
	asPrivate, err := e.ephemeralFn()
	if err != nil {
		return nil, fmt.Errorf("webpush generate ephemeral key: %w", err)
	}

	return encryptRecord(plaintext, uaPublic, authSecret, salt, asPrivate)
}

// encryptRecord builds the single aes128gcm record described by RFC 8291
// section 3 and frames it per RFC 8188 section 2.1.
func encryptRecord(plaintext, uaPublic, authSecret, salt []byte, asPrivate *ecdh.PrivateKey) ([]byte, error) {
	if len(plaintext) > MaxPayloadLength {
		return nil, fmt.Errorf("webpush payload of %d bytes exceeds the maximum of %d", len(plaintext), MaxPayloadLength)
	}
	if len(salt) != saltLength {
		return nil, fmt.Errorf("webpush salt must be %d bytes, got %d", saltLength, len(salt))
	}
	if len(authSecret) != authSecretLength {
		return nil, fmt.Errorf("webpush auth secret must be %d bytes, got %d", authSecretLength, len(authSecret))
	}

	uaPub, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, fmt.Errorf("webpush parse subscription p256dh key: %w", err)
	}
	ecdhSecret, err := asPrivate.ECDH(uaPub)
	if err != nil {
		return nil, fmt.Errorf("webpush ECDH: %w", err)
	}

	asPublic := asPrivate.PublicKey().Bytes()
	keys, err := deriveContentKeys(ecdhSecret, authSecret, salt, uaPublic, asPublic)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keys.cek)
	if err != nil {
		return nil, fmt.Errorf("webpush new AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("webpush new GCM: %w", err)
	}

	padded := make([]byte, 0, len(plaintext)+1)
	padded = append(padded, plaintext...)
	padded = append(padded, paddingDelimiter)

	body := make([]byte, 0, headerLength+len(padded)+gcmTagLength)
	body = append(body, salt...)
	body = binary.BigEndian.AppendUint32(body, recordSize)
	body = append(body, byte(len(asPublic)))
	body = append(body, asPublic...)

	// The record sequence number is zero for the only record, so NONCE is used
	// unmodified rather than XORed with a counter.
	return aead.Seal(body, keys.nonce, padded, nil), nil
}

// contentKeys holds every step of the RFC 8291 section 3.4 derivation. The
// intermediates are retained so tests can pin them against appendix A.
type contentKeys struct {
	prkKey []byte
	ikm    []byte
	prk    []byte
	cek    []byte
	nonce  []byte
}

func deriveContentKeys(ecdhSecret, authSecret, salt, uaPublic, asPublic []byte) (contentKeys, error) {
	var keys contentKeys

	prkKey, err := hkdf.Extract(sha256.New, ecdhSecret, authSecret)
	if err != nil {
		return keys, fmt.Errorf("webpush derive PRK_key: %w", err)
	}
	keys.prkKey = prkKey

	keyInfo := make([]byte, 0, len(webPushInfo)+len(uaPublic)+len(asPublic))
	keyInfo = append(keyInfo, webPushInfo...)
	keyInfo = append(keyInfo, uaPublic...)
	keyInfo = append(keyInfo, asPublic...)

	ikm, err := hkdf.Expand(sha256.New, prkKey, string(keyInfo), sha256.Size)
	if err != nil {
		return keys, fmt.Errorf("webpush derive IKM: %w", err)
	}
	keys.ikm = ikm

	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return keys, fmt.Errorf("webpush derive PRK: %w", err)
	}
	keys.prk = prk

	cek, err := hkdf.Expand(sha256.New, prk, cekInfo, 16)
	if err != nil {
		return keys, fmt.Errorf("webpush derive CEK: %w", err)
	}
	keys.cek = cek

	nonce, err := hkdf.Expand(sha256.New, prk, nonceInfo, 12)
	if err != nil {
		return keys, fmt.Errorf("webpush derive NONCE: %w", err)
	}
	keys.nonce = nonce

	return keys, nil
}

func randomSalt() ([]byte, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

func generateEphemeralKey() (*ecdh.PrivateKey, error) {
	return ecdh.P256().GenerateKey(rand.Reader)
}

// decodeKey accepts both base64url and standard base64, padded or not, because
// subscriptions reach us through browsers and stores that disagree on the alphabet.
func decodeKey(name, value string, wantLen int) ([]byte, error) {
	if value == "" {
		return nil, fmt.Errorf("webpush subscription %s key is empty", name)
	}
	decoded, err := decodeBase64(value)
	if err != nil {
		return nil, fmt.Errorf("webpush decode subscription %s key: %w", name, err)
	}
	if len(decoded) != wantLen {
		return nil, fmt.Errorf("webpush subscription %s key must be %d bytes, got %d", name, wantLen, len(decoded))
	}
	return decoded, nil
}

var base64URLReplacer = strings.NewReplacer("+", "-", "/", "_")

func decodeBase64(value string) ([]byte, error) {
	normalised := base64URLReplacer.Replace(strings.TrimRight(value, "="))
	return base64.RawURLEncoding.DecodeString(normalised)
}
