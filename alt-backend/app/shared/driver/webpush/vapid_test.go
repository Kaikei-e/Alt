package webpush

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"
)

// Any valid P-256 scalar works as an ES256 key; reuse the RFC 8291 sender key so
// the tests carry no generated secrets.
const testVAPIDPrivateB64 = rfcASPrivateB64

func newTestSigner(t *testing.T, subject string) *VAPIDSigner {
	t.Helper()
	signer, err := NewVAPIDSigner(subject, testVAPIDPrivateB64)
	if err != nil {
		t.Fatalf("NewVAPIDSigner: %v", err)
	}
	return signer
}

func TestNewVAPIDSigner_SubjectNormalisation(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		want    string
		wantErr bool
	}{
		{name: "bare email gains one mailto scheme", subject: "you@example.com", want: "mailto:you@example.com"},
		{name: "already schemed mailto is left alone", subject: "mailto:you@example.com", want: "mailto:you@example.com"},
		{name: "https subject is left alone", subject: "https://example.com/contact", want: "https://example.com/contact"},
		// Guards the webpush-go bug: an unconditional prefix yields "mailto:mailto:..."
		// which Apple rejects with 403 BadJwtToken.
		{name: "double mailto collapses to one", subject: "mailto:mailto:you@example.com", want: "mailto:you@example.com"},
		{name: "surrounding whitespace trimmed", subject: "  you@example.com  ", want: "mailto:you@example.com"},
		{name: "uppercase scheme normalised", subject: "MAILTO:you@example.com", want: "mailto:you@example.com"},
		{name: "empty subject rejected", subject: "", wantErr: true},
		{name: "not an email and not a url rejected", subject: "not-a-subject", wantErr: true},
		{name: "unsupported scheme rejected", subject: "ftp://example.com", wantErr: true},
		{name: "http scheme rejected", subject: "http://example.com/contact", wantErr: true},
		{name: "empty mailto rejected", subject: "mailto:", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signer, err := NewVAPIDSigner(tt.subject, testVAPIDPrivateB64)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewVAPIDSigner(%q) = nil error, want error", tt.subject)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewVAPIDSigner(%q): %v", tt.subject, err)
			}
			if got := signer.Subject(); got != tt.want {
				t.Errorf("Subject() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewVAPIDSigner_InvalidPrivateKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"not base64", "!!!!"},
		{"too short", base64.RawURLEncoding.EncodeToString([]byte("short"))},
		{"zero scalar", base64.RawURLEncoding.EncodeToString(make([]byte, 32))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewVAPIDSigner("mailto:you@example.com", tt.key); err == nil {
				t.Fatal("expected an error for an invalid private key")
			}
		})
	}
}

func TestVAPIDSigner_Audience(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
		wantErr  bool
	}{
		{name: "fcm path is dropped", endpoint: "https://fcm.googleapis.com/fcm/send/dGhpcy1pcy1hLXRlc3Q", want: "https://fcm.googleapis.com"},
		{name: "mozilla path is dropped", endpoint: "https://updates.push.services.mozilla.com/wpush/v2/gAAAAAB", want: "https://updates.push.services.mozilla.com"},
		{name: "apple path is dropped", endpoint: "https://web.push.apple.com/QAtR7DUxQ", want: "https://web.push.apple.com"},
		{name: "query and fragment are dropped", endpoint: "https://push.example.net/push/abc?token=1#frag", want: "https://push.example.net"},
		{name: "explicit port is kept", endpoint: "https://push.example.net:8443/push/abc", want: "https://push.example.net:8443"},
		{name: "root path", endpoint: "https://push.example.net/", want: "https://push.example.net"},
		{name: "empty endpoint rejected", endpoint: "", wantErr: true},
		{name: "relative endpoint rejected", endpoint: "/push/abc", wantErr: true},
		{name: "missing host rejected", endpoint: "https:///push/abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := audienceOf(tt.endpoint)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("audienceOf(%q) = %q, want error", tt.endpoint, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("audienceOf(%q): %v", tt.endpoint, err)
			}
			if got != tt.want {
				t.Errorf("audienceOf(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}

// vapidHeaderParts splits "vapid t=<jwt>, k=<key>" into its two values.
func vapidHeaderParts(t *testing.T, header string) (jwt, key string) {
	t.Helper()
	scheme, params, ok := strings.Cut(header, " ")
	if !ok || scheme != "vapid" {
		t.Fatalf("header %q must start with the vapid auth scheme", header)
	}
	tokenPart, keyPart, ok := strings.Cut(params, ", ")
	if !ok {
		t.Fatalf("header %q must contain %q between its parameters", header, ", ")
	}
	jwt, ok = strings.CutPrefix(tokenPart, "t=")
	if !ok {
		t.Fatalf("header %q must carry t=", header)
	}
	key, ok = strings.CutPrefix(keyPart, "k=")
	if !ok {
		t.Fatalf("header %q must carry k=", header)
	}
	return jwt, key
}

func TestVAPIDSigner_AuthorizationHeader(t *testing.T) {
	signer := newTestSigner(t, "you@example.com")
	issuedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	signer.nowFn = func() time.Time { return issuedAt }

	header, err := signer.AuthorizationHeader("https://fcm.googleapis.com/fcm/send/abc")
	if err != nil {
		t.Fatalf("AuthorizationHeader: %v", err)
	}

	jwt, key := vapidHeaderParts(t, header)

	if key != signer.PublicKey() {
		t.Errorf("k = %q, want the signer public key %q", key, signer.PublicKey())
	}
	if decoded := mustDecodeB64(t, key); len(decoded) != publicKeyLength {
		t.Errorf("k decodes to %d bytes, want %d (uncompressed point)", len(decoded), publicKeyLength)
	}

	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT has %d parts, want 3", len(parts))
	}

	var jwtHeader struct {
		Typ string `json:"typ"`
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(mustDecodeB64(t, parts[0]), &jwtHeader); err != nil {
		t.Fatalf("decode JWT header: %v", err)
	}
	if jwtHeader.Typ != "JWT" || jwtHeader.Alg != "ES256" {
		t.Errorf("JWT header = %+v, want {typ:JWT alg:ES256}", jwtHeader)
	}

	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(mustDecodeB64(t, parts[1]), &claims); err != nil {
		t.Fatalf("decode JWT claims: %v", err)
	}
	if want := "https://fcm.googleapis.com"; claims.Aud != want {
		t.Errorf("aud = %q, want %q (scheme and host only, no path)", claims.Aud, want)
	}
	if want := "mailto:you@example.com"; claims.Sub != want {
		t.Errorf("sub = %q, want %q", claims.Sub, want)
	}
	if want := issuedAt.Add(vapidTokenLifetime).Unix(); claims.Exp != want {
		t.Errorf("exp = %d, want %d", claims.Exp, want)
	}
	if lifetime := time.Duration(claims.Exp-issuedAt.Unix()) * time.Second; lifetime > 24*time.Hour {
		t.Errorf("exp is %v ahead, RFC 8292 caps it at 24h and Apple rejects more than a day", lifetime)
	}

	// A DER/ASN.1 signature is the classic cause of an unexplained 403, so the
	// raw R||S width is asserted before the signature is verified.
	sig := mustDecodeB64(t, parts[2])
	if len(sig) != 64 {
		t.Fatalf("signature is %d bytes, want a fixed-width 64-byte R||S pair", len(sig))
	}

	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), mustDecodeB64(t, key))
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		t.Error("signature does not verify against the advertised public key")
	}
}

// TestVAPIDSigner_SignatureFixedWidth catches the intermittent failure mode where
// an R or S value with leading zero bytes is emitted short instead of padded.
func TestVAPIDSigner_SignatureFixedWidth(t *testing.T) {
	signer := newTestSigner(t, "you@example.com")
	for i := range 200 {
		// A distinct audience per iteration forces a fresh signature past the cache.
		header, err := signer.AuthorizationHeader(fmt.Sprintf("https://push%d.example.net/push/abc", i))
		if err != nil {
			t.Fatalf("AuthorizationHeader %d: %v", i, err)
		}
		jwt, _ := vapidHeaderParts(t, header)
		parts := strings.Split(jwt, ".")
		if sig := mustDecodeB64(t, parts[2]); len(sig) != 64 {
			t.Fatalf("iteration %d: signature is %d bytes, want 64", i, len(sig))
		}
	}
}

func TestVAPIDSigner_CachesPerAudience(t *testing.T) {
	signer := newTestSigner(t, "you@example.com")
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	signer.nowFn = func() time.Time { return now }

	fcmFirst, err := signer.AuthorizationHeader("https://fcm.googleapis.com/fcm/send/abc")
	if err != nil {
		t.Fatalf("AuthorizationHeader: %v", err)
	}
	fcmSecond, err := signer.AuthorizationHeader("https://fcm.googleapis.com/fcm/send/different-endpoint")
	if err != nil {
		t.Fatalf("AuthorizationHeader: %v", err)
	}
	if fcmFirst != fcmSecond {
		t.Error("the same audience should reuse the cached token")
	}

	apple, err := signer.AuthorizationHeader("https://web.push.apple.com/abc")
	if err != nil {
		t.Fatalf("AuthorizationHeader: %v", err)
	}
	if apple == fcmFirst {
		t.Error("a different audience must get its own token")
	}

	// Just under half the lifetime: still cached.
	now = now.Add(vapidTokenLifetime/2 - time.Minute)
	stillCached, err := signer.AuthorizationHeader("https://fcm.googleapis.com/fcm/send/abc")
	if err != nil {
		t.Fatalf("AuthorizationHeader: %v", err)
	}
	if stillCached != fcmFirst {
		t.Error("token refreshed before half its lifetime elapsed")
	}

	// Past half the lifetime: regenerated.
	now = now.Add(2 * time.Minute)
	refreshed, err := signer.AuthorizationHeader("https://fcm.googleapis.com/fcm/send/abc")
	if err != nil {
		t.Fatalf("AuthorizationHeader: %v", err)
	}
	if refreshed == fcmFirst {
		t.Error("token should be regenerated once half its lifetime has elapsed")
	}
}

func TestVAPIDSigner_ConcurrentUse(t *testing.T) {
	signer := newTestSigner(t, "you@example.com")
	endpoints := []string{
		"https://fcm.googleapis.com/fcm/send/abc",
		"https://updates.push.services.mozilla.com/wpush/v2/abc",
		"https://web.push.apple.com/abc",
	}

	var wg sync.WaitGroup
	for i := range 60 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := signer.AuthorizationHeader(endpoints[i%len(endpoints)]); err != nil {
				t.Errorf("AuthorizationHeader: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestVAPIDSigner_RejectsBadEndpoint(t *testing.T) {
	signer := newTestSigner(t, "you@example.com")
	if _, err := signer.AuthorizationHeader("not a url"); err == nil {
		t.Fatal("expected an error for an unparseable endpoint")
	}
}
