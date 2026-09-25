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
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/smallstep/cli-utils/token"
	"github.com/smallstep/cli-utils/token/provision"
	"go.step.sm/crypto/jose"
	"go.step.sm/crypto/keyutil"
	"go.step.sm/crypto/randutil"
	"go.step.sm/crypto/x509util"
)

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

// mintOTT creates a signed one-time token using provisioner credentials for CSR submission.
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

// createCSR generates a new private key and corresponding PEM-encoded certificate signing request.
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

// rootFingerprint retrieves the root CA fingerprint via health endpoint TLS verification.
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

// loadProvisionerJWK fetches and decrypts the provisioner private key from step-ca.
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

// postSign posts a JSON signing request to the target step-ca endpoint.
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

// readCapped reads up to max bytes from reader to prevent unbounded memory consumption.
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

// joinURL combines a base URL and path segment correctly.
func joinURL(base, path string) string {
	u, err := url.Parse(base)
	if err != nil {
		return strings.TrimRight(base, "/") + path
	}
	return u.ResolveReference(&url.URL{Path: path}).String()
}

// newCATransport builds an HTTP transport enforcing TLS 1.3 and HTTP/2 for CA communication.
func newCATransport(cfg *tls.Config) *http.Transport {
	return &http.Transport{
		TLSClientConfig:   cfg,
		ForceAttemptHTTP2: true,
	}
}

// readProvisionerPassword reads and validates provisioner password from a secure file.
func readProvisionerPassword(path string) ([]byte, error) {
	raw, err := readRegularNoFollow(path, maxPasswordBytes)
	if err != nil {
		if strings.Contains(err.Error(), "exceeds") {
			return nil, ErrPasswordTooLarge
		}
		return nil, ErrPasswordUnreadable
	}
	password := bytes.TrimSpace(raw)
	if len(password) == 0 {
		return nil, ErrPasswordEmpty
	}
	return password, nil
}

// zeroBytes overwrites sensitive secret byte slices in memory.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
