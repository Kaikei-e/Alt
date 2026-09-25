package pki

import (
	"bytes"
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.step.sm/crypto/jose"
	"go.step.sm/crypto/x509util"
)

const (
	defaultIssueTimeout = 15 * time.Second
	ottLifetime         = 5 * time.Minute
)

// NativeStepCAIssuer is the production step-ca client for in-process
// enrollment. Official APIs used:
//   - go.step.sm/crypto/jose — JWE unwrap of the subject-scoped JWK
//   - github.com/smallstep/cli-utils/token/provision — short-lived single-use OTT
//   - go.step.sm/crypto/{keyutil,pemutil,x509util} — local key + CSR
//
// github.com/smallstep/certificates/ca is not imported: that package compiles
// the CA server and transitively pulls github.com/jackc/pgx/v5, which
// cmd/backend and cmd/harvester must not link (ADR-000954). The HTTP contract
// (GET /provisioners, POST /sign + OTT, POST /rekey + mTLS) is the same one
// the official ca.Client uses.
type NativeStepCAIssuer struct {
	CAURL        string
	RootFile     string
	Provisioner  string
	PasswordFile string
	Timeout      time.Duration

	mu             sync.Mutex
	cred           *provisionerCred
	transport      *http.Transport
	extraTransport []*http.Transport
}

type provisionerCred struct {
	name        string
	jwk         *jose.JSONWebKey
	fingerprint string
	audience    string
}

// requestTimeout computes the effective timeout bounded by context deadline.
func (s *NativeStepCAIssuer) requestTimeout(ctx context.Context) time.Duration {
	d := s.Timeout
	if d <= 0 {
		d = defaultIssueTimeout
	}
	if deadline, ok := ctx.Deadline(); ok {
		remain := time.Until(deadline)
		if remain <= 0 {
			return time.Millisecond
		}
		if remain < d {
			return remain
		}
	}
	return d
}

// Issue requests a newly minted leaf certificate and private key using a single-use OTT.
func (s *NativeStepCAIssuer) Issue(ctx context.Context, subject string, sans []string) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("pki: issue: %w", err)
	}
	if err := s.guardProvisioner(); err != nil {
		return nil, nil, err
	}
	if len(sans) == 0 {
		sans = []string{subject}
	}
	cred, err := s.credentials(ctx)
	if err != nil {
		return nil, nil, err
	}
	ott, err := mintOTT(cred, subject, sans)
	if err != nil {
		return nil, nil, err
	}
	csrPEM, key, err := createCSR(subject, sans)
	if err != nil {
		return nil, nil, err
	}
	client, err := s.httpClient(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := postSign(ctx, client, joinURL(s.CAURL, "/sign"), signRequestJSON{CSR: csrPEM, OTT: ott})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, fmt.Errorf("pki: sign: %w", ctxErr)
		}
		return nil, nil, fmt.Errorf("pki: sign: %w", err)
	}
	return s.validateAndEncode(subject, sans, resp, key)
}

// Rekey asks step-ca to sign a new key using the still-valid leaf as client
// authentication. Expired certificates must use Issue (OTT + /sign).
func (s *NativeStepCAIssuer) Rekey(ctx context.Context, certPEM, keyPEM []byte, subject string, sans []string) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("pki: rekey: %w", err)
	}
	if err := s.guardProvisioner(); err != nil {
		return nil, nil, err
	}
	if len(sans) == 0 {
		sans = []string{subject}
	}
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("pki: parse current cert for rekey: %w", err)
	}
	csrPEM, key, err := createCSR(subject, sans)
	if err != nil {
		return nil, nil, err
	}
	client, err := s.httpClient(ctx, &tlsCert)
	if err != nil {
		return nil, nil, err
	}
	resp, err := postSign(ctx, client, joinURL(s.CAURL, "/rekey"), rekeyRequestJSON{CSR: csrPEM})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, fmt.Errorf("pki: rekey: %w", ctxErr)
		}
		return nil, nil, fmt.Errorf("pki: rekey: %w", err)
	}
	return s.validateAndEncode(subject, sans, resp, key)
}

// guardProvisioner ensures the provisioner name and password file do not collide with shared root credentials.
func (s *NativeStepCAIssuer) guardProvisioner() error {
	if s.Provisioner == "pki-agent" || s.Provisioner == "" {
		return fmt.Errorf("%w (got %q)", ErrSharedProvisioner, s.Provisioner)
	}
	if strings.Contains(s.PasswordFile, "step_ca_root_password") {
		return ErrSharedRootSecret
	}
	return nil
}

// credentials lazily resolves and caches the provisioner JWK and root fingerprint.
func (s *NativeStepCAIssuer) credentials(ctx context.Context) (*provisionerCred, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("pki: load provisioner: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cred != nil {
		return s.cred, nil
	}
	password, err := readProvisionerPassword(s.PasswordFile)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(password)

	client, err := s.httpClientLocked(ctx, nil)
	if err != nil {
		return nil, err
	}
	fp, err := rootFingerprint(ctx, client, s.CAURL)
	if err != nil {
		return nil, err
	}
	jwk, err := loadProvisionerJWK(ctx, client, s.CAURL, s.Provisioner, password)
	if err != nil {
		return nil, err
	}
	s.cred = &provisionerCred{
		name:        s.Provisioner,
		jwk:         jwk,
		fingerprint: fp,
		audience:    joinURL(s.CAURL, "/1.0/sign"),
	}
	return s.cred, nil
}

// httpClient returns an HTTP client configured with pinned CA root and optional client certificate.
func (s *NativeStepCAIssuer) httpClient(ctx context.Context, clientCert *tls.Certificate) (*http.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.httpClientLocked(ctx, clientCert)
}

// httpClientLocked constructs an HTTP client while holding the issuer lock.
func (s *NativeStepCAIssuer) httpClientLocked(ctx context.Context, clientCert *tls.Certificate) (*http.Client, error) {
	if err := requireHTTPS(s.CAURL); err != nil {
		return nil, err
	}
	tr, err := s.transportLocked(clientCert)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout:   s.requestTimeout(ctx),
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return ErrRedirect
		},
	}, nil
}

// transportLocked returns a shared or cloned TLS transport pinned to the CA root pool.
func (s *NativeStepCAIssuer) transportLocked(clientCert *tls.Certificate) (*http.Transport, error) {
	pool, err := s.rootPool()
	if err != nil {
		return nil, err
	}
	if clientCert == nil {
		if s.transport == nil {
			s.transport = newCATransport(&tls.Config{
				MinVersion: tls.VersionTLS13,
				RootCAs:    pool,
			})
		}
		return s.transport, nil
	}
	cloned := newCATransport(&tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      pool,
		Certificates: []tls.Certificate{*clientCert},
	})
	s.extraTransport = append(s.extraTransport, cloned)
	return cloned, nil
}

// rootPool parses and returns the pinned CA root certificate pool.
func (s *NativeStepCAIssuer) rootPool() (*x509.CertPool, error) {
	data, err := readRegularNoFollow(s.RootFile, maxRootPEMBytes)
	if err != nil {
		return nil, fmt.Errorf("pki: read CA root: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("pki: parse CA root %q: no certificates", s.RootFile)
	}
	return pool, nil
}

// CloseIdleConnections releases idle HTTP connections on the reused
// no-client-cert transport and any rekey transports.
func (s *NativeStepCAIssuer) CloseIdleConnections() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}
	for _, tr := range s.extraTransport {
		tr.CloseIdleConnections()
	}
}

// validateAndEncode validates the sign response against the pinned root and encodes PEM outputs.
func (s *NativeStepCAIssuer) validateAndEncode(subject string, sans []string, resp *signResponseJSON, key crypto.PrivateKey) ([]byte, []byte, error) {
	if resp == nil || strings.TrimSpace(resp.Crt) == "" {
		return nil, nil, fmt.Errorf("pki: empty sign response")
	}
	leaf, err := parseCertPEM(resp.Crt)
	if err != nil {
		return nil, nil, fmt.Errorf("pki: parse issued cert: %w", err)
	}
	if leaf.Subject.CommonName != subject {
		return nil, nil, fmt.Errorf("pki: issued CN %q does not match subject %q", leaf.Subject.CommonName, subject)
	}
	if err := certMatchesSANs(leaf, sans); err != nil {
		return nil, nil, err
	}
	if err := publicKeyMatches(leaf, key); err != nil {
		return nil, nil, err
	}
	rootPEM, err := readRegularNoFollow(s.RootFile, maxRootPEMBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("pki: load pinned CA root: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(rootPEM) {
		return nil, nil, fmt.Errorf("pki: load pinned CA root %q: no certificates", s.RootFile)
	}
	intermediates := x509.NewCertPool()
	var chain []*x509.Certificate
	for _, pemStr := range resp.CertChain {
		c, err := parseCertPEM(pemStr)
		if err != nil {
			return nil, nil, fmt.Errorf("pki: parse issued chain: %w", err)
		}
		chain = append(chain, c)
		if !bytes.Equal(c.Raw, leaf.Raw) {
			intermediates.AddCert(c)
		}
	}
	if caPEM := strings.TrimSpace(resp.CA); caPEM != "" {
		c, err := parseCertPEM(caPEM)
		if err != nil {
			return nil, nil, fmt.Errorf("pki: parse issued CA: %w", err)
		}
		if !bytes.Equal(c.Raw, leaf.Raw) {
			intermediates.AddCert(c)
			chain = append(chain, c)
		}
	}
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	if wantDNS, _, _, _ := x509util.SplitSANs(sans); len(wantDNS) > 0 {
		opts.DNSName = wantDNS[0]
	}
	if _, err := leaf.Verify(opts); err != nil {
		return nil, nil, fmt.Errorf("pki: issued chain does not verify against pinned CA root: %w", err)
	}
	if err := requireDualEKU(leaf); err != nil {
		return nil, nil, err
	}
	return encodeIssued(leaf, chain, key)
}
