package pki

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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

func requireDualEKU(leaf *x509.Certificate) error {
	var client, server bool
	for _, u := range leaf.ExtKeyUsage {
		switch u {
		case x509.ExtKeyUsageClientAuth:
			client = true
		case x509.ExtKeyUsageServerAuth:
			server = true
		}
	}
	if !client || !server {
		return fmt.Errorf("pki: issued cert must have both clientAuth and serverAuth EKUs")
	}
	return nil
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
		return "", fmt.Errorf("pki: health body: %w", err)
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
			return nil, classifyCAStatus(resp.StatusCode)
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
		return nil, classifyCAStatus(resp.StatusCode)
	}
	var out signResponseJSON
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("pki: decode sign response: %w", err)
	}
	return &out, nil
}

func classifyCAStatus(code int) error {
	if code >= 500 {
		return fmt.Errorf("%w (status %d)", ErrCAUnavailable, code)
	}
	return fmt.Errorf("%w (status %d)", ErrCARejected, code)
}

func readCapped(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, ErrResponseTooLarge
	}
	return data, nil
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

func certMatchesSANs(cert *x509.Certificate, sans []string) error {
	wantDNS, wantIP, wantEmail, wantURI := x509util.SplitSANs(sans)
	if !sameFoldedSet(cert.DNSNames, wantDNS) {
		return fmt.Errorf("pki: issued DNS SAN set mismatch: got %q want %q", cert.DNSNames, wantDNS)
	}
	if !sameIPSet(cert.IPAddresses, wantIP) {
		return fmt.Errorf("pki: issued IP SAN set mismatch: got %v want %v", cert.IPAddresses, wantIP)
	}
	if !sameFoldedSet(cert.EmailAddresses, wantEmail) {
		return fmt.Errorf("pki: issued email SAN set mismatch: got %q want %q", cert.EmailAddresses, wantEmail)
	}
	if !sameURISet(cert.URIs, wantURI) {
		return fmt.Errorf("pki: issued URI SAN set mismatch")
	}
	return nil
}

func sameFoldedSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[string]int, len(want))
	for _, w := range want {
		counts[strings.ToLower(w)]++
	}
	for _, g := range got {
		k := strings.ToLower(g)
		if counts[k] == 0 {
			return false
		}
		counts[k]--
	}
	return true
}

func sameIPSet(got, want []net.IP) bool {
	if len(got) != len(want) {
		return false
	}
	used := make([]bool, len(want))
	for _, g := range got {
		matched := false
		for i, w := range want {
			if used[i] {
				continue
			}
			if g.Equal(w) {
				used[i] = true
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func sameURISet(got, want []*url.URL) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[string]int, len(want))
	for _, w := range want {
		if w == nil {
			return false
		}
		counts[w.String()]++
	}
	for _, g := range got {
		if g == nil {
			return false
		}
		k := g.String()
		if counts[k] == 0 {
			return false
		}
		counts[k]--
	}
	return true
}

func publicKeyMatches(leaf *x509.Certificate, key crypto.PrivateKey) error {
	signer, ok := key.(crypto.Signer)
	if !ok {
		return fmt.Errorf("pki: issued key is not a signer")
	}
	eq, ok := leaf.PublicKey.(interface{ Equal(crypto.PublicKey) bool })
	if !ok || !eq.Equal(signer.Public()) {
		return fmt.Errorf("pki: issued leaf public key does not match CSR key")
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
			return nil, ErrPasswordTooLarge
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
