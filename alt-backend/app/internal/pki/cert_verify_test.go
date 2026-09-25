package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"
)

func TestClassifyCAStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    error
	}{
		{name: "internal server error", statusCode: 500, wantErr: ErrCAUnavailable},
		{name: "bad gateway", statusCode: 502, wantErr: ErrCAUnavailable},
		{name: "service unavailable", statusCode: 503, wantErr: ErrCAUnavailable},
		{name: "gateway timeout", statusCode: 504, wantErr: ErrCAUnavailable},
		{name: "bad request", statusCode: 400, wantErr: ErrCARejected},
		{name: "unauthorized", statusCode: 401, wantErr: ErrCARejected},
		{name: "forbidden", statusCode: 403, wantErr: ErrCARejected},
		{name: "not found", statusCode: 404, wantErr: ErrCARejected},
		{name: "too many requests", statusCode: 429, wantErr: ErrCARejected},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyCAStatus(tt.statusCode)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("classifyCAStatus(%d) = %v, want error wrapping %v", tt.statusCode, err, tt.wantErr)
			}
		})
	}
}

func TestPublicKeyMatches(t *testing.T) {
	key1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key1.PublicKey, key1)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		key     any
		wantErr bool
	}{
		{name: "matching key", key: key1, wantErr: false},
		{name: "mismatched key", key: key2, wantErr: true},
		{name: "non-signer key", key: "not-a-private-key", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := publicKeyMatches(leaf, tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("publicKeyMatches() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestCertMatchesSANs(t *testing.T) {
	uri1, _ := url.Parse("https://alt.local/service")
	uri2, _ := url.Parse("https://alt.local/other")

	cert := &x509.Certificate{
		DNSNames:       []string{"alt-backend.local", "api.local"},
		IPAddresses:    []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1")},
		EmailAddresses: []string{"ops@alt.local"},
		URIs:           []*url.URL{uri1},
	}

	tests := []struct {
		name    string
		sans    []string
		wantErr bool
	}{
		{
			name:    "exact match",
			sans:    []string{"alt-backend.local", "api.local", "127.0.0.1", "10.0.0.1", "ops@alt.local", uri1.String()},
			wantErr: false,
		},
		{
			name:    "case folded dns and email",
			sans:    []string{"ALT-BACKEND.LOCAL", "API.LOCAL", "127.0.0.1", "10.0.0.1", "OPS@ALT.LOCAL", uri1.String()},
			wantErr: false,
		},
		{
			name:    "missing one dns",
			sans:    []string{"alt-backend.local", "127.0.0.1", "10.0.0.1", "ops@alt.local", uri1.String()},
			wantErr: true,
		},
		{
			name:    "extra dns",
			sans:    []string{"alt-backend.local", "api.local", "extra.local", "127.0.0.1", "10.0.0.1", "ops@alt.local", uri1.String()},
			wantErr: true,
		},
		{
			name:    "mismatched ip",
			sans:    []string{"alt-backend.local", "api.local", "127.0.0.2", "10.0.0.1", "ops@alt.local", uri1.String()},
			wantErr: true,
		},
		{
			name:    "mismatched uri",
			sans:    []string{"alt-backend.local", "api.local", "127.0.0.1", "10.0.0.1", "ops@alt.local", uri2.String()},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := certMatchesSANs(cert, tt.sans)
			if (err != nil) != tt.wantErr {
				t.Fatalf("certMatchesSANs() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
