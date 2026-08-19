package pki

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smallstep/cli-utils/token"
	"github.com/smallstep/cli-utils/token/provision"
	"go.step.sm/crypto/jose"
	"go.step.sm/crypto/keyutil"
	"go.step.sm/crypto/pemutil"
	"go.step.sm/crypto/randutil"
	"go.step.sm/crypto/x509util"
)

const (
	defaultIssueTimeout = 15 * time.Second
	ottLifetime         = 5 * time.Minute

	stepCAPBES2Alg       = "PBES2-HS256+A128KW"
	stepCAPBES2Enc       = "A256GCM"
	stepCAPBES2P2C       = 600000
	maxCompactJWEBytes   = 16 << 10
	maxDecryptedJWKBytes = 8 << 10
	maxJWEHeaderBytes    = 1024
)

// NativeStepCAIssuer is the production step-ca client for in-process
// enrollment. Official APIs used:
//   - go.step.sm/crypto/jose — JWE unwrap of the subject-scoped JWK
//   - github.com/smallstep/cli-utils/token/provision — short-lived single-use OTT
//   - go.step.sm/crypto/{keyutil,pemutil,x509util} — local key + CSR
//
// github.com/smallstep/certificates/ca is not imported: that package compiles
// the CA server. The HTTP contract (GET /provisioners, POST /sign + OTT,
// POST /rekey + mTLS) is the same one the official ca.Client uses.
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

func (s *NativeStepCAIssuer) requireHTTPS() error {
	u, err := url.Parse(s.CAURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%w (got %q)", ErrNotHTTPS, s.CAURL)
	}
	return nil
}

func (s *NativeStepCAIssuer) withOpDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	d := maxOpTimeout
	if s.Timeout > 0 && s.Timeout < d {
		d = s.Timeout
	}
	return context.WithTimeout(ctx, d)
}

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

func (s *NativeStepCAIssuer) Issue(ctx context.Context, subject string, sans []string) ([]byte, []byte, error) {
	ctx, cancel := s.withOpDeadline(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("pki: issue: %w", err)
	}
	if err := s.guardProvisioner(); err != nil {
		return nil, nil, err
	}
	if err := s.requireHTTPS(); err != nil {
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
	ctx, cancel := s.withOpDeadline(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("pki: rekey: %w", err)
	}
	if err := s.guardProvisioner(); err != nil {
		return nil, nil, err
	}
	if err := s.requireHTTPS(); err != nil {
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

func (s *NativeStepCAIssuer) guardProvisioner() error {
	if s.Provisioner == "pki-agent" || s.Provisioner == "" {
		return fmt.Errorf("%w (got %q)", ErrSharedProvisioner, s.Provisioner)
	}
	if strings.Contains(s.PasswordFile, "step_ca_root_password") {
		return fmt.Errorf("%w (got %q)", ErrSharedRootSecret, s.PasswordFile)
	}
	return nil
}

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

func (s *NativeStepCAIssuer) httpClient(ctx context.Context, clientCert *tls.Certificate) (*http.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.httpClientLocked(ctx, clientCert)
}

func (s *NativeStepCAIssuer) httpClientLocked(ctx context.Context, clientCert *tls.Certificate) (*http.Client, error) {
	if err := s.requireHTTPS(); err != nil {
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

func newCATransport(cfg *tls.Config) *http.Transport {
	return &http.Transport{
		TLSClientConfig:   cfg,
		ForceAttemptHTTP2: true,
	}
}

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
	if err := certHasExactSANs(leaf, sans); err != nil {
		return nil, nil, err
	}
	if err := leafPublicKeyMatches(leaf, key); err != nil {
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
	if _, err := leaf.Verify(x509.VerifyOptions{
		DNSName:       subject,
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return nil, nil, fmt.Errorf("pki: issued chain does not verify against pinned CA root: %w", err)
	}
	if err := requireDualEKU(leaf); err != nil {
		return nil, nil, err
	}
	return encodeIssued(leaf, chain, key)
}

type signRequestJSON struct {
	CSR string `json:"csr"`
	OTT string `json:"ott"`
}

type rekeyRequestJSON struct {
	CSR string `json:"csr"`
}

type signResponseJSON struct {
	Crt       string   `json:"crt"`
	CA        string   `json:"ca"`
	CertChain []string `json:"certChain"`
}

type provisionersJSON struct {
	Provisioners []struct {
		Type         string `json:"type"`
		Name         string `json:"name"`
		EncryptedKey string `json:"encryptedKey"`
	} `json:"provisioners"`
	NextCursor string `json:"nextCursor"`
}

func mintOTT(cred *provisionerCred, subject string, sans []string) (string, error) {
	jwtID, err := randutil.Hex(64)
	if err != nil {
		return "", fmt.Errorf("pki: ott jti: %w", err)
	}
	notBefore := time.Now()
	opts := []token.Options{
		token.WithJWTID(jwtID),
		token.WithKid(cred.jwk.KeyID),
		token.WithIssuer(cred.name),
		token.WithAudience(cred.audience),
		token.WithValidity(notBefore, notBefore.Add(ottLifetime)),
		token.WithSANS(sans),
	}
	if cred.fingerprint != "" {
		opts = append(opts, token.WithSHA(cred.fingerprint))
	}
	tok, err := provision.New(subject, opts...)
	if err != nil {
		return "", fmt.Errorf("pki: mint ott: %w", err)
	}
	alg := string(cred.jwk.Algorithm)
	if alg == "" {
		alg = string(jose.ES256)
	}
	ott, err := tok.SignedString(alg, cred.jwk.Key)
	if err != nil {
		return "", fmt.Errorf("pki: sign ott: %w", err)
	}
	return ott, nil
}

func createCSR(subject string, sans []string) (string, crypto.PrivateKey, error) {
	key, err := keyutil.GenerateDefaultKey()
	if err != nil {
		return "", nil, fmt.Errorf("pki: generate key: %w", err)
	}
	dnsNames, ips, emails, uris := x509util.SplitSANs(sans)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:        pkix.Name{CommonName: subject},
		DNSNames:       dnsNames,
		IPAddresses:    ips,
		EmailAddresses: emails,
		URIs:           uris,
	}, key)
	if err != nil {
		return "", nil, fmt.Errorf("pki: create csr: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})), key, nil
}

func rootFingerprint(ctx context.Context, client *http.Client, caURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(caURL, "/health"), nil)
	if err != nil {
		return "", fmt.Errorf("pki: health request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("pki: health: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := readCapped(resp.Body, maxResponseBytes); err != nil {
		return "", fmt.Errorf("pki: health: %w", err)
	}
	if resp.TLS == nil || len(resp.TLS.VerifiedChains) == 0 {
		return "", fmt.Errorf("pki: health: missing verified TLS chain")
	}
	chain := resp.TLS.VerifiedChains[len(resp.TLS.VerifiedChains)-1]
	if len(chain) == 0 {
		return "", fmt.Errorf("pki: health: empty verified TLS chain")
	}
	return x509util.Fingerprint(chain[len(chain)-1]), nil
}

func loadProvisionerJWK(ctx context.Context, client *http.Client, caURL, name string, password []byte) (*jose.JSONWebKey, error) {
	var cursor string
	for page := 0; page < maxProvisionerPages; page++ {
		u, err := url.Parse(joinURL(caURL, "/provisioners"))
		if err != nil {
			return nil, fmt.Errorf("pki: provisioners url: %w", err)
		}
		q := u.Query()
		q.Set("limit", "100")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("pki: provisioners request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("pki: provisioners: %w", err)
		}
		body, err := readCapped(resp.Body, maxResponseBytes)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("pki: read provisioners: %w", err)
		}
		if resp.StatusCode >= 400 {
			return nil, caStatusError(resp.StatusCode)
		}
		var list provisionersJSON
		if err := json.Unmarshal(body, &list); err != nil {
			return nil, fmt.Errorf("pki: decode provisioners: %w", err)
		}
		for _, p := range list.Provisioners {
			if !strings.EqualFold(p.Type, "JWK") || p.Name != name || p.EncryptedKey == "" {
				continue
			}
			jwk, err := decryptProvisionerJWK(p.EncryptedKey, password)
			if err == nil {
				return jwk, nil
			}
		}
		if list.NextCursor == "" {
			return nil, fmt.Errorf("pki: jwk provisioner %q not found (or password is wrong)", name)
		}
		cursor = list.NextCursor
	}
	return nil, ErrProvisionerPageLimit
}

func decryptProvisionerJWK(encryptedKey string, password []byte) (*jose.JSONWebKey, error) {
	if err := inspectJWEHeader(encryptedKey); err != nil {
		return nil, err
	}
	enc, err := jose.ParseEncrypted(encryptedKey)
	if err != nil {
		return nil, fmt.Errorf("pki: parse provisioner jwe: %w", err)
	}
	data, err := enc.Decrypt(password)
	if err != nil {
		return nil, fmt.Errorf("pki: decrypt provisioner jwk: %w", err)
	}
	if len(data) > maxDecryptedJWKBytes {
		return nil, fmt.Errorf("pki: decrypted provisioner JWK exceeded size cap")
	}
	jwk := new(jose.JSONWebKey)
	if err := json.Unmarshal(data, jwk); err != nil {
		return nil, fmt.Errorf("pki: unmarshal provisioner jwk: %w", err)
	}
	return jwk, nil
}

func inspectJWEHeader(compact string) error {
	if len(compact) > maxCompactJWEBytes {
		return fmt.Errorf("pki: provisioner JWE exceeded size cap")
	}
	parts := strings.Split(compact, ".")
	if len(parts) != 5 {
		return fmt.Errorf("pki: malformed provisioner jwe")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		padded := parts[0] + strings.Repeat("=", (4-len(parts[0])%4)%4)
		raw, err = base64.URLEncoding.DecodeString(padded)
		if err != nil {
			return fmt.Errorf("pki: malformed provisioner jwe header")
		}
	}
	if len(raw) > maxJWEHeaderBytes {
		return fmt.Errorf("pki: provisioner JWE header exceeded size cap")
	}
	var header map[string]any
	if err := json.Unmarshal(raw, &header); err != nil {
		return fmt.Errorf("pki: malformed provisioner jwe header")
	}
	if _, ok := header["zip"]; ok {
		return fmt.Errorf("pki: unexpected provisioner JWE zip")
	}
	allowed := map[string]struct{}{
		"alg": {}, "enc": {}, "p2c": {}, "p2s": {}, "kid": {}, "typ": {}, "cty": {},
	}
	for k := range header {
		if _, ok := allowed[k]; !ok {
			return fmt.Errorf("pki: unexpected provisioner JWE header %q", k)
		}
	}
	if header["alg"] != stepCAPBES2Alg || header["enc"] != stepCAPBES2Enc {
		return fmt.Errorf("pki: unexpected provisioner JWE alg/enc")
	}
	p2c, ok := jsonNumber(header["p2c"])
	if !ok || p2c != stepCAPBES2P2C {
		return fmt.Errorf("pki: unexpected provisioner JWE p2c")
	}
	return nil
}

func jsonNumber(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		i := int(n)
		return i, float64(i) == n
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	case int:
		return n, true
	case int64:
		return int(n), true
	default:
		if s, ok := v.(string); ok {
			i, err := strconv.Atoi(s)
			return i, err == nil
		}
		return 0, false
	}
}

func requireDualEKU(cert *x509.Certificate) error {
	var client, server bool
	for _, u := range cert.ExtKeyUsage {
		if u == x509.ExtKeyUsageClientAuth {
			client = true
		}
		if u == x509.ExtKeyUsageServerAuth {
			server = true
		}
	}
	if !client || !server {
		return fmt.Errorf("pki: issued cert missing dual EKU clientAuth+serverAuth")
	}
	return nil
}

func postSign(ctx context.Context, client *http.Client, endpoint string, payload any) (*signResponseJSON, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("pki: marshal sign request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("pki: sign request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := readCapped(resp.Body, maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("pki: read sign response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, caStatusError(resp.StatusCode)
	}
	var out signResponseJSON
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("pki: decode sign response: %w", err)
	}
	return &out, nil
}

func caStatusError(status int) error {
	sentinel := ErrCARejected
	if status >= 500 {
		sentinel = ErrCAUnavailable
	}
	return fmt.Errorf("%w (status %d)", sentinel, status)
}

func readCapped(r io.Reader, capBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, capBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > capBytes {
		return nil, ErrResponseTooLarge
	}
	return raw, nil
}

func joinURL(base, path string) string {
	u, err := url.Parse(base)
	if err != nil {
		return strings.TrimRight(base, "/") + path
	}
	return u.ResolveReference(&url.URL{Path: path}).String()
}

func parseCertPEM(s string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, fmt.Errorf("pki: no certificate pem")
	}
	return x509.ParseCertificate(block.Bytes)
}

func certHasExactSANs(cert *x509.Certificate, sans []string) error {
	wantDNS, wantIPs, wantEmails, wantURIs := x509util.SplitSANs(sans)
	if err := equalFoldSet("DNS SAN", cert.DNSNames, wantDNS); err != nil {
		return err
	}
	if err := equalIPSet(cert.IPAddresses, wantIPs); err != nil {
		return err
	}
	if err := equalFoldSet("email SAN", cert.EmailAddresses, wantEmails); err != nil {
		return err
	}
	gotURI := make([]string, 0, len(cert.URIs))
	for _, u := range cert.URIs {
		if u != nil {
			gotURI = append(gotURI, u.String())
		}
	}
	wantURI := make([]string, 0, len(wantURIs))
	for _, u := range wantURIs {
		if u != nil {
			wantURI = append(wantURI, u.String())
		}
	}
	return equalFoldSet("URI SAN", gotURI, wantURI)
}

func equalFoldSet(kind string, got, want []string) error {
	g := make(map[string]struct{}, len(got))
	for _, v := range got {
		g[strings.ToLower(v)] = struct{}{}
	}
	w := make(map[string]struct{}, len(want))
	for _, v := range want {
		w[strings.ToLower(v)] = struct{}{}
	}
	if len(g) != len(w) {
		return fmt.Errorf("pki: issued cert %s set mismatch: got %v want %v", kind, got, want)
	}
	for k := range w {
		if _, ok := g[k]; !ok {
			return fmt.Errorf("pki: issued cert missing %s %q", kind, k)
		}
	}
	return nil
}

func equalIPSet(got, want []net.IP) error {
	g := make(map[string]struct{}, len(got))
	for _, ip := range got {
		g[ip.String()] = struct{}{}
	}
	w := make(map[string]struct{}, len(want))
	for _, ip := range want {
		w[ip.String()] = struct{}{}
	}
	if len(g) != len(w) {
		return fmt.Errorf("pki: issued cert IP SAN set mismatch: got %v want %v", got, want)
	}
	for k := range w {
		if _, ok := g[k]; !ok {
			return fmt.Errorf("pki: issued cert missing IP SAN %q", k)
		}
	}
	return nil
}

func leafPublicKeyMatches(leaf *x509.Certificate, key crypto.PrivateKey) error {
	type hasPublic interface {
		Public() crypto.PublicKey
	}
	priv, ok := key.(hasPublic)
	if !ok {
		return fmt.Errorf("pki: generated key has no public key")
	}
	want := priv.Public()
	got, ok := leaf.PublicKey.(interface{ Equal(crypto.PublicKey) bool })
	if !ok || !got.Equal(want) {
		return fmt.Errorf("pki: issued leaf public key does not match generated key")
	}
	return nil
}

func encodeIssued(leaf *x509.Certificate, chain []*x509.Certificate, key crypto.PrivateKey) ([]byte, []byte, error) {
	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw}); err != nil {
		return nil, nil, fmt.Errorf("pki: encode cert: %w", err)
	}
	seen := map[string]struct{}{string(leaf.Raw): {}}
	for _, c := range chain {
		if c == nil {
			continue
		}
		if _, ok := seen[string(c.Raw)]; ok {
			continue
		}
		seen[string(c.Raw)] = struct{}{}
		if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}); err != nil {
			return nil, nil, fmt.Errorf("pki: encode chain: %w", err)
		}
	}
	block, err := pemutil.Serialize(key)
	if err != nil {
		return nil, nil, fmt.Errorf("pki: serialize key: %w", err)
	}
	certPEM := certBuf.Bytes()
	keyPEM := pem.EncodeToMemory(block)
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return nil, nil, fmt.Errorf("pki: issued cert/key pair is not usable: %w", err)
	}
	return certPEM, keyPEM, nil
}

func readProvisionerPassword(path string) ([]byte, error) {
	raw, err := readRegularNoFollow(path, maxPasswordBytes)
	if err != nil {
		if strings.Contains(err.Error(), "exceeds") {
			return nil, fmt.Errorf("%w: %v", ErrPasswordTooLarge, err)
		}
		return nil, fmt.Errorf("pki: provisioner password file: %w", err)
	}
	password := bytes.TrimSpace(raw)
	if len(password) == 0 {
		return nil, fmt.Errorf("pki: provisioner password file %q is empty", path)
	}
	return password, nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
