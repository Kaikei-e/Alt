package webpush

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"encoding/base64"
	"math"
	"strings"
	"testing"
)

// Test vectors from RFC 8291 section 5 and appendix A.
const (
	rfcPlaintextB64  = "V2hlbiBJIGdyb3cgdXAsIEkgd2FudCB0byBiZSBhIHdhdGVybWVsb24"
	rfcSaltB64       = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcAuthSecretB64 = "BTBZMqHH6r4Tts7J_aSIgg"

	rfcASPublicB64  = "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8"
	rfcASPrivateB64 = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	rfcUAPublicB64  = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcUAPrivateB64 = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"

	rfcECDHSecretB64 = "kyrL1jIIOHEzg3sM2ZWRHDRB62YACZhhSlknJ672kSs"
	rfcPRKKeyB64     = "Snr3JMxaHVDXHWJn5wdC52WjpCtd2EIEGBykDcZW32k"
	rfcIKMB64        = "S4lYMb_L0FxCeq0WhDx813KgSYqU26kOyzWUdsXYyrg"
	rfcPRKB64        = "09_eUZGrsvxChDCGRCdkLiDXrReGOEVeSCdCcPBSJSc"
	rfcCEKB64        = "oIhVW04MRdy2XN9CiKLxTg"
	rfcNonceB64      = "4h_95klXJ5E_qnoN"

	// The 86-octet RFC 8188 header: salt(16) || rs(4) || idlen(1) || as_public(65).
	rfcHeaderB64 = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8"
	// The complete encrypted body from RFC 8291 section 5.
	rfcBodyB64 = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
)

func mustDecodeB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return b
}

func rfcSubscription() Subscription {
	return Subscription{
		Endpoint: "https://push.example.net/push/JzLQ3raZJfFBR0aqvOMsLrt54w4rJUsV",
		Keys: SubscriptionKeys{
			P256dh: rfcUAPublicB64,
			Auth:   rfcAuthSecretB64,
		},
	}
}

// TestEncryptRecord_RFC8291Vector is the anchor test: with the salt and the
// application-server keypair pinned to the RFC's values, the encrypted body must
// reproduce RFC 8291 section 5 byte for byte.
func TestEncryptRecord_RFC8291Vector(t *testing.T) {
	asPriv, err := ecdh.P256().NewPrivateKey(mustDecodeB64(t, rfcASPrivateB64))
	if err != nil {
		t.Fatalf("parse application server private key: %v", err)
	}

	got, err := encryptRecord(
		mustDecodeB64(t, rfcPlaintextB64),
		mustDecodeB64(t, rfcUAPublicB64),
		mustDecodeB64(t, rfcAuthSecretB64),
		mustDecodeB64(t, rfcSaltB64),
		asPriv,
	)
	if err != nil {
		t.Fatalf("encryptRecord: %v", err)
	}

	want := mustDecodeB64(t, rfcBodyB64)
	if !bytes.Equal(got, want) {
		t.Errorf("encrypted body mismatch\n got: %s\nwant: %s",
			base64.RawURLEncoding.EncodeToString(got),
			base64.RawURLEncoding.EncodeToString(want))
	}

	// Localise a mismatch to the header framing vs. the ciphertext.
	wantHeader := mustDecodeB64(t, rfcHeaderB64)
	if len(got) >= len(wantHeader) && !bytes.Equal(got[:len(wantHeader)], wantHeader) {
		t.Errorf("header mismatch\n got: %x\nwant: %x", got[:len(wantHeader)], wantHeader)
	}
	if len(got) != len(want) {
		t.Errorf("body length = %d, want %d", len(got), len(want))
	}
}

func TestRFC8291IDLen_RejectsOverflow(t *testing.T) {
	n := math.MaxUint8 + 1
	got, err := rfc8291IDLen(n)
	if err == nil {
		t.Fatalf("rfc8291IDLen(%d) succeeded with byte %d; overflow must be rejected so the idlen octet does not wrap", n, got)
	}
}

func TestRFC8291IDLen_AcceptsP256UncompressedLen(t *testing.T) {
	got, err := rfc8291IDLen(publicKeyLength)
	if err != nil {
		t.Fatalf("rfc8291IDLen(%d): %v", publicKeyLength, err)
	}
	if got != byte(publicKeyLength) {
		t.Errorf("idlen = %d, want %d", got, publicKeyLength)
	}
}

func TestRFC8291IDLen_RejectsNegative(t *testing.T) {
	if _, err := rfc8291IDLen(-1); err == nil {
		t.Fatal("rfc8291IDLen(-1) succeeded; negative length must be rejected")
	}
}

// TestDeriveContentKeys_RFC8291Vector pins every intermediate value from RFC 8291
// appendix A so a derivation regression names the exact step that broke.
func TestDeriveContentKeys_RFC8291Vector(t *testing.T) {
	asPriv, err := ecdh.P256().NewPrivateKey(mustDecodeB64(t, rfcASPrivateB64))
	if err != nil {
		t.Fatalf("parse application server private key: %v", err)
	}
	uaPub, err := ecdh.P256().NewPublicKey(mustDecodeB64(t, rfcUAPublicB64))
	if err != nil {
		t.Fatalf("parse user agent public key: %v", err)
	}

	ecdhSecret, err := asPriv.ECDH(uaPub)
	if err != nil {
		t.Fatalf("ECDH: %v", err)
	}
	if want := mustDecodeB64(t, rfcECDHSecretB64); !bytes.Equal(ecdhSecret, want) {
		t.Fatalf("ecdh_secret = %s, want %s",
			base64.RawURLEncoding.EncodeToString(ecdhSecret), rfcECDHSecretB64)
	}

	keys, err := deriveContentKeys(
		ecdhSecret,
		mustDecodeB64(t, rfcAuthSecretB64),
		mustDecodeB64(t, rfcSaltB64),
		mustDecodeB64(t, rfcUAPublicB64),
		asPriv.PublicKey().Bytes(),
	)
	if err != nil {
		t.Fatalf("deriveContentKeys: %v", err)
	}

	tests := []struct {
		name string
		got  []byte
		want string
	}{
		{"PRK_key", keys.prkKey, rfcPRKKeyB64},
		{"IKM", keys.ikm, rfcIKMB64},
		{"PRK", keys.prk, rfcPRKB64},
		{"CEK", keys.cek, rfcCEKB64},
		{"NONCE", keys.nonce, rfcNonceB64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base64.RawURLEncoding.EncodeToString(tt.got); got != tt.want {
				t.Errorf("%s = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

// TestEncrypt_RoundTrip checks that a body built with a freshly generated ephemeral
// key and random salt decrypts back to the original plaintext with the user agent's
// private key -- the property the RFC vector alone cannot cover.
func TestEncrypt_RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty", ""},
		{"short", "hello"},
		{"json", `{"title":"Alt","body":"new articles"}`},
		{"max size", strings.Repeat("x", MaxPayloadLength)},
	}

	enc := NewEncrypter()
	uaPriv, err := ecdh.P256().NewPrivateKey(mustDecodeB64(t, rfcUAPrivateB64))
	if err != nil {
		t.Fatalf("parse user agent private key: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := enc.Encrypt(rfcSubscription(), []byte(tt.plaintext))
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			got, err := decryptForTest(t, body, uaPriv, mustDecodeB64(t, rfcAuthSecretB64))
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if string(got) != tt.plaintext {
				t.Errorf("round trip = %q, want %q", got, tt.plaintext)
			}
		})
	}
}

// TestEncrypt_UsesFreshSaltAndEphemeralKey guards against a regression where the
// production defaults are wired to a constant, which would reuse a nonce across
// messages and break AES-GCM's security entirely.
func TestEncrypt_UsesFreshSaltAndEphemeralKey(t *testing.T) {
	enc := NewEncrypter()
	sub := rfcSubscription()

	first, err := enc.Encrypt(sub, []byte("same plaintext"))
	if err != nil {
		t.Fatalf("Encrypt first: %v", err)
	}
	second, err := enc.Encrypt(sub, []byte("same plaintext"))
	if err != nil {
		t.Fatalf("Encrypt second: %v", err)
	}

	if bytes.Equal(first[:saltLength], second[:saltLength]) {
		t.Error("salt reused across sends")
	}
	if bytes.Equal(first[headerLength-publicKeyLength:headerLength], second[headerLength-publicKeyLength:headerLength]) {
		t.Error("ephemeral public key reused across sends")
	}
	if bytes.Equal(first, second) {
		t.Error("identical ciphertext for repeated send")
	}
}

func TestEncrypt_PayloadLengthLimit(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{"at limit", MaxPayloadLength, false},
		{"one over limit", MaxPayloadLength + 1, true},
		{"far over limit", MaxPayloadLength * 2, true},
	}

	enc := NewEncrypter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := enc.Encrypt(rfcSubscription(), bytes.Repeat([]byte("a"), tt.size))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Encrypt(%d bytes) = nil error, want error", tt.size)
				}
				if !strings.Contains(err.Error(), "payload") {
					t.Errorf("error %q should mention the payload", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Encrypt(%d bytes): %v", tt.size, err)
			}
		})
	}
}

func TestEncrypt_InvalidSubscriptionKeys(t *testing.T) {
	tests := []struct {
		name   string
		p256dh string
		auth   string
	}{
		{"empty p256dh", "", rfcAuthSecretB64},
		{"empty auth", rfcUAPublicB64, ""},
		{"p256dh not base64url", "!!!not base64!!!", rfcAuthSecretB64},
		{"auth not base64url", rfcUAPublicB64, "!!!not base64!!!"},
		{"p256dh wrong length", base64.RawURLEncoding.EncodeToString([]byte("short")), rfcAuthSecretB64},
		{"auth wrong length", rfcUAPublicB64, base64.RawURLEncoding.EncodeToString([]byte("short"))},
		{"p256dh not on curve", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{4}, publicKeyLength)), rfcAuthSecretB64},
		{"p256dh compressed form", base64.RawURLEncoding.EncodeToString(append([]byte{2}, bytes.Repeat([]byte{1}, 32)...)), rfcAuthSecretB64},
	}

	enc := NewEncrypter()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := Subscription{
				Endpoint: "https://push.example.net/push/abc",
				Keys:     SubscriptionKeys{P256dh: tt.p256dh, Auth: tt.auth},
			}
			if _, err := enc.Encrypt(sub, []byte("payload")); err == nil {
				t.Fatal("expected an error for an invalid subscription key")
			}
		})
	}
}

// TestEncrypt_AcceptsPaddedBase64 covers subscriptions stored with standard-padded
// base64, which browsers and intermediate stores both emit in the wild.
func TestEncrypt_AcceptsPaddedBase64(t *testing.T) {
	uaPub := mustDecodeB64(t, rfcUAPublicB64)
	auth := mustDecodeB64(t, rfcAuthSecretB64)

	sub := Subscription{
		Endpoint: "https://push.example.net/push/abc",
		Keys: SubscriptionKeys{
			P256dh: base64.StdEncoding.EncodeToString(uaPub),
			Auth:   base64.StdEncoding.EncodeToString(auth),
		},
	}
	if _, err := NewEncrypter().Encrypt(sub, []byte("payload")); err != nil {
		t.Fatalf("Encrypt with padded standard base64: %v", err)
	}
}

// decryptForTest reverses encryptRecord using the receiver's private key.
func decryptForTest(t *testing.T, body []byte, uaPriv *ecdh.PrivateKey, authSecret []byte) ([]byte, error) {
	t.Helper()
	if len(body) < headerLength {
		t.Fatalf("body too short: %d", len(body))
	}
	salt := body[:saltLength]
	if idLen := body[saltLength+4]; int(idLen) != publicKeyLength {
		t.Fatalf("keyid length = %d, want %d", idLen, publicKeyLength)
	}
	asPublic := body[saltLength+5 : headerLength]

	asPub, err := ecdh.P256().NewPublicKey(asPublic)
	if err != nil {
		t.Fatalf("parse application server public key: %v", err)
	}
	ecdhSecret, err := uaPriv.ECDH(asPub)
	if err != nil {
		t.Fatalf("ECDH: %v", err)
	}

	keys, err := deriveContentKeys(ecdhSecret, authSecret, salt, uaPriv.PublicKey().Bytes(), asPublic)
	if err != nil {
		t.Fatalf("deriveContentKeys: %v", err)
	}

	block, err := aes.NewCipher(keys.cek)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("cipher.NewGCM: %v", err)
	}
	padded, err := aead.Open(nil, keys.nonce, body[headerLength:], nil)
	if err != nil {
		return nil, err
	}
	idx := bytes.LastIndexByte(padded, paddingDelimiter)
	if idx < 0 {
		t.Fatal("padding delimiter not found")
	}
	return padded[:idx], nil
}
