package authhub

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

const maxIntrospectResponseBytes = 64 * 1024

type certReloader struct {
	certPath string
	keyPath  string
	mu       sync.Mutex
	cert     *tls.Certificate
	certMod  time.Time
	keyMod   time.Time
}

func newCertReloader(certPath, keyPath string) (*certReloader, error) {
	r := &certReloader{certPath: certPath, keyPath: keyPath}
	if _, err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *certReloader) load() (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	certStat, err := os.Stat(r.certPath)
	if err != nil {
		if r.cert != nil {
			return r.cert, nil
		}
		return nil, fmt.Errorf("stat cert %q: %w", r.certPath, err)
	}
	keyStat, err := os.Stat(r.keyPath)
	if err != nil {
		if r.cert != nil {
			return r.cert, nil
		}
		return nil, fmt.Errorf("stat key %q: %w", r.keyPath, err)
	}

	if r.cert != nil && !certStat.ModTime().After(r.certMod) && !keyStat.ModTime().After(r.keyMod) {
		return r.cert, nil
	}

	cert, err := tls.LoadX509KeyPair(r.certPath, r.keyPath)
	if err != nil {
		if r.cert != nil {
			return r.cert, nil
		}
		return nil, fmt.Errorf("load x509 keypair: %w", err)
	}

	r.cert = &cert
	r.certMod = certStat.ModTime()
	r.keyMod = keyStat.ModTime()
	return r.cert, nil
}

// Config holds primitive configuration for RemoteUserJWTVerifier.
type Config struct {
	URL      string
	CertPath string
	KeyPath  string
	CAPath   string
	Timeout  time.Duration
}

// RemoteUserJWTVerifier validates user delegation tokens via mTLS introspection to auth-hub.
type RemoteUserJWTVerifier struct {
	url      string
	certPath string
	keyPath  string
	caPath   string
	client   *http.Client
}

// URL returns the introspection endpoint URL.
func (v *RemoteUserJWTVerifier) URL() string { return v.url }

// CertPath returns the client certificate path.
func (v *RemoteUserJWTVerifier) CertPath() string { return v.certPath }

// KeyPath returns the client private key path.
func (v *RemoteUserJWTVerifier) KeyPath() string { return v.keyPath }

// CAPath returns the CA bundle path.
func (v *RemoteUserJWTVerifier) CAPath() string { return v.caPath }

// NewRemoteUserJWTVerifier constructs a RemoteUserJWTVerifier with given config.
func NewRemoteUserJWTVerifier(cfg Config) (*RemoteUserJWTVerifier, error) {
	if cfg.URL == "" {
		return nil, errors.New("remote JWT introspection URL is required")
	}

	parsedURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse introspection URL: %w", err)
	}
	if parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("introspection URL must use strict HTTPS scheme, got %q", parsedURL.Scheme)
	}

	if cfg.CertPath == "" {
		cfg.CertPath = "/certs/svc-cert.pem"
	}
	if cfg.KeyPath == "" {
		cfg.KeyPath = "/certs/svc-key.pem"
	}
	if cfg.CAPath == "" {
		cfg.CAPath = "/trust/ca-bundle.pem"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	reloader, err := newCertReloader(cfg.CertPath, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("load mTLS client cert/key (%s, %s): %w", cfg.CertPath, cfg.KeyPath, err)
	}

	caCert, err := os.ReadFile(filepath.Clean(cfg.CAPath))
	if err != nil {
		return nil, fmt.Errorf("read CA bundle %q: %w", cfg.CAPath, err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certs from %q", cfg.CAPath)
	}

	serverName := parsedURL.Hostname()

	tlsConfig := &tls.Config{
		RootCAs:    caCertPool,
		ServerName: serverName,
		MinVersion: tls.VersionTLS13,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return reloader.load()
		},
	}

	transport := &http.Transport{
		TLSClientConfig:   tlsConfig,
		ForceAttemptHTTP2: true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errors.New("http redirects are prohibited during token introspection to prevent JWT leakage")
		},
	}

	return &RemoteUserJWTVerifier{
		url:      cfg.URL,
		certPath: cfg.CertPath,
		keyPath:  cfg.KeyPath,
		caPath:   cfg.CAPath,
		client:   client,
	}, nil
}

type introspectReq struct {
	Token string `json:"token"`
}

type introspectResp struct {
	Active   bool   `json:"active"`
	Sub      string `json:"sub"`
	TenantID string `json:"tenant_id"`
	Exp      int64  `json:"exp"`
}

// AuthHubClaims is the driver DTO.
type AuthHubClaims struct {
	Active   bool
	Sub      string
	TenantID string
	Exp      int64
}

// ValidateToken performs remote introspection over mTLS.
func (v *RemoteUserJWTVerifier) ValidateToken(ctx context.Context, tokenStr string) (*AuthHubClaims, error) {
	if tokenStr == "" {
		return nil, errors.New("missing user delegation token")
	}

	reqBody, err := json.Marshal(introspectReq{Token: tokenStr})
	if err != nil {
		return nil, fmt.Errorf("marshal introspection request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create introspection request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspection request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspection returned status %d", resp.StatusCode)
	}

	// Complete bounded response read max + 1
	lr := io.LimitReader(resp.Body, int64(maxIntrospectResponseBytes)+1)
	body, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("read introspection response: %w", err)
	}
	if len(body) > maxIntrospectResponseBytes {
		return nil, fmt.Errorf("introspection response exceeded maximum allowed size (%d bytes)", maxIntrospectResponseBytes)
	}

	// Strict single JSON decode and ensure EOF (reject junk, second object, etc.)
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var ir introspectResp
	if err := dec.Decode(&ir); err != nil {
		return nil, fmt.Errorf("decode introspection response: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, errors.New("introspection response contains trailing garbage or multiple JSON objects")
	}

	if !ir.Active {
		return nil, errors.New("token is inactive")
	}
	if ir.Sub == "" {
		return nil, errors.New("token subject is empty")
	}
	if _, err := uuid.Parse(ir.Sub); err != nil {
		return nil, fmt.Errorf("invalid token subject (must be valid UUID): %w", err)
	}
	if ir.TenantID == "" {
		return nil, errors.New("token tenant_id is empty")
	}
	if _, err := uuid.Parse(ir.TenantID); err != nil {
		return nil, fmt.Errorf("invalid token tenant_id (must be valid UUID): %w", err)
	}
	if ir.Exp <= time.Now().Unix() {
		return nil, errors.New("token is expired")
	}

	return &AuthHubClaims{
		Active:   ir.Active,
		Sub:      ir.Sub,
		TenantID: ir.TenantID,
		Exp:      ir.Exp,
	}, nil
}
