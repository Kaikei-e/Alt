package pki

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.step.sm/crypto/jose"
)

type ottClaims struct {
	jose.Claims
	SANs []string `json:"sans"`
	SHA  string   `json:"sha"`
}

type recordedReq struct {
	Method string
	Path   string
	Body   []byte
}

type fakeStepCA struct {
	t                   *testing.T
	password            []byte
	provisioner         string
	jwk                 *jose.JSONWebKey
	encKey              string
	caCert              *x509.Certificate
	caKey               *ecdsa.PrivateKey
	mu                  sync.Mutex
	usedJTI             map[string]struct{}
	seen                []recordedReq
	signDelay           time.Duration
	blockSign           chan struct{}
	malformedSign       bool
	mutateLeaf          func(*x509.Certificate)
	rejectExpired       bool
	rejectReuse         bool
	signStatus          int
	signBody            []byte
	holdHealth          chan struct{}
	redirectSign        int
	endlessProvisioners bool
}

func encryptJWKFast(t *testing.T, jwk *jose.JSONWebKey, password []byte) string {
	t.Helper()
	raw, err := json.Marshal(jwk)
	if err != nil {
		t.Fatal(err)
	}
	salt := make([]byte, jose.PBKDF2SaltSize)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	encrypter, err := jose.NewEncrypter(jose.DefaultEncAlgorithm, jose.Recipient{
		Algorithm:  jose.PBES2_HS256_A128KW,
		Key:        password,
		PBES2Count: stepCAPBES2P2C,
		PBES2Salt:  salt,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	jwe, err := encrypter.Encrypt(raw)
	if err != nil {
		t.Fatal(err)
	}
	out, err := jwe.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func generateTestCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "alt-test-step-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func newFakeStepCA(t *testing.T, provisioner string, password []byte) *fakeStepCA {
	t.Helper()
	jwk, err := jose.GenerateJWK("EC", "P-256", "ES256", "sig", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	caCert, caKey := generateTestCA(t)
	return &fakeStepCA{
		t:             t,
		password:      append([]byte(nil), password...),
		provisioner:   provisioner,
		jwk:           jwk,
		encKey:        encryptJWKFast(t, jwk, password),
		caCert:        caCert,
		caKey:         caKey,
		usedJTI:       map[string]struct{}{},
		rejectExpired: true,
		rejectReuse:   true,
	}
}

func (f *fakeStepCA) start(t *testing.T) (caURL, rootFile string) {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(f.serveHTTP))
	srv.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{f.caCert.Raw},
			PrivateKey:  f.caKey,
		}},
		ClientAuth: tls.RequestClientCert,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	rootFile = filepath.Join(t.TempDir(), "root.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.caCert.Raw})
	if err := os.WriteFile(rootFile, pemBytes, 0o444); err != nil {
		t.Fatal(err)
	}
	return srv.URL, rootFile
}

func (f *fakeStepCA) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	f.mu.Lock()
	f.seen = append(f.seen, recordedReq{Method: r.Method, Path: r.URL.Path, Body: body})
	f.mu.Unlock()
}

func (f *fakeStepCA) lastSign() recordedReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.seen) - 1; i >= 0; i-- {
		if f.seen[i].Path == "/sign" || f.seen[i].Path == "/1.0/sign" {
			return f.seen[i]
		}
	}
	return recordedReq{}
}

func (f *fakeStepCA) lastRekey() recordedReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.seen) - 1; i >= 0; i-- {
		if f.seen[i].Path == "/rekey" {
			return f.seen[i]
		}
	}
	return recordedReq{}
}

func (f *fakeStepCA) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.record(r)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/health":
		if f.holdHealth != nil {
			<-f.holdHealth
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/provisioners":
		f.writeProvisioners(w)
	case r.Method == http.MethodPost && (r.URL.Path == "/sign" || r.URL.Path == "/1.0/sign"):
		if f.redirectSign != 0 {
			http.Redirect(w, r, "/elsewhere", f.redirectSign)
			return
		}
		f.handleSign(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/rekey":
		f.handleRekey(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/renew":
		http.Error(w, "renew requires a valid client certificate; expired leaves must re-enroll", http.StatusUnauthorized)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeStepCA) writeProvisioners(w http.ResponseWriter) {
	if f.endlessProvisioners {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"provisioners":[],"nextCursor":"next"}`))
		return
	}
	pub := f.jwk.Public()
	keyJSON, err := json.Marshal(&pub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"provisioners":[{"type":"JWK","name":` + jsonString(f.provisioner) + `,"key":` + string(keyJSON) + `,"encryptedKey":` + jsonString(f.encKey) + `}]}`))
}

func summarizeSeen(seen []recordedReq) string {
	var b strings.Builder
	for i, r := range seen {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s %s (%d bytes)", r.Method, r.Path, len(r.Body))
	}
	return b.String()
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func writeCAError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "message": msg})
}

func (f *fakeStepCA) handleSign(w http.ResponseWriter, r *http.Request) {
	if f.signDelay > 0 {
		timer := time.NewTimer(f.signDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			writeCAError(w, http.StatusGatewayTimeout, "canceled")
			return
		}
	}
	if f.blockSign != nil {
		select {
		case <-f.blockSign:
		case <-r.Context().Done():
			writeCAError(w, http.StatusGatewayTimeout, "canceled")
			return
		}
	}
	if f.malformedSign {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{not-json`))
		return
	}
	if len(f.signBody) > 0 {
		if f.signStatus == 0 {
			f.signStatus = http.StatusCreated
		}
		w.WriteHeader(f.signStatus)
		_, _ = w.Write(f.signBody)
		return
	}
	var req struct {
		CSR string `json:"csr"`
		OTT string `json:"ott"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeCAError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	claims, err := f.verifyOTT(req.OTT)
	if err != nil {
		writeCAError(w, http.StatusUnauthorized, "ott: "+err.Error())
		return
	}
	csr, err := parseCSRPEMerr(req.CSR)
	if err != nil {
		writeCAError(w, http.StatusBadRequest, "csr: "+err.Error())
		return
	}
	if csr.Subject.CommonName != claims.Subject {
		writeCAError(w, http.StatusBadRequest, "csr subject mismatch")
		return
	}
	leaf := f.signCSR(csr, func(c *x509.Certificate) {
		if f.mutateLeaf != nil {
			f.mutateLeaf(c)
		}
	})
	f.writeSignResponse(w, leaf)
}

func (f *fakeStepCA) handleRekey(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "missing client certificate", http.StatusBadRequest)
		return
	}
	peer := r.TLS.PeerCertificates[0]
	if time.Now().After(peer.NotAfter) {
		http.Error(w, "expired client certificate", http.StatusUnauthorized)
		return
	}
	var req struct {
		CSR string `json:"csr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	csr := parseCSRPEM(f.t, req.CSR)
	leaf := f.signCSR(csr, nil)
	f.writeSignResponse(w, leaf)
}

func (f *fakeStepCA) verifyOTT(ott string) (ottClaims, error) {
	var claims ottClaims
	tok, err := jose.ParseSigned(ott)
	if err != nil {
		return claims, err
	}
	pub := f.jwk.Public()
	if err := tok.Claims(pub.Key, &claims); err != nil {
		return claims, err
	}
	if claims.Issuer != f.provisioner {
		return claims, errors.New("wrong issuer")
	}
	if !audienceHasSign(claims.Audience) {
		return claims, errors.New("wrong audience")
	}
	if f.rejectExpired && claims.Expiry != nil && time.Now().After(claims.Expiry.Time()) {
		return claims, errors.New("expired ott")
	}
	if claims.ID == "" {
		return claims, errors.New("missing jti")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rejectReuse {
		if _, ok := f.usedJTI[claims.ID]; ok {
			return claims, errors.New("reused ott")
		}
		f.usedJTI[claims.ID] = struct{}{}
	}
	return claims, nil
}

func (f *fakeStepCA) verifyOTTForTest(ott string) (ottClaims, error) {
	tok, err := jose.ParseSigned(ott)
	if err != nil {
		return ottClaims{}, err
	}
	var claims ottClaims
	pub := f.jwk.Public()
	if err := tok.Claims(pub.Key, &claims); err != nil {
		return ottClaims{}, err
	}
	return claims, nil
}

func audienceHasSign(aud jose.Audience) bool {
	for _, a := range aud {
		if strings.Contains(a, "/1.0/sign") {
			return true
		}
	}
	return false
}

func parseCSRPEM(t *testing.T, s string) *x509.CertificateRequest {
	t.Helper()
	csr, err := parseCSRPEMerr(s)
	if err != nil {
		t.Fatal(err)
	}
	return csr
}

func parseCSRPEMerr(s string) (*x509.CertificateRequest, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, errors.New("csr pem")
	}
	return x509.ParseCertificateRequest(block.Bytes)
}

func (f *fakeStepCA) signCSR(csr *x509.CertificateRequest, mutate func(*x509.Certificate)) *x509.Certificate {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		f.t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:   serial,
		Subject:        csr.Subject,
		DNSNames:       csr.DNSNames,
		IPAddresses:    csr.IPAddresses,
		EmailAddresses: csr.EmailAddresses,
		URIs:           csr.URIs,
		NotBefore:      time.Now().Add(-time.Minute),
		NotAfter:       time.Now().Add(time.Hour),
		KeyUsage:       x509.KeyUsageDigitalSignature,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	if mutate != nil {
		mutate(tmpl)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, f.caCert, csr.PublicKey, f.caKey)
	if err != nil {
		f.t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		f.t.Fatal(err)
	}
	return cert
}

func (f *fakeStepCA) writeSignResponse(w http.ResponseWriter, leaf *x509.Certificate) {
	leafPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw}))
	caPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.caCert.Raw}))
	body := map[string]any{
		"crt":       leafPEM,
		"ca":        caPEM,
		"certChain": []string{leafPEM, caPEM},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(raw)
}

func writePasswordFile(t *testing.T, dir, name, password string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(password+"\n"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func newNativeIssuer(t *testing.T, ca *fakeStepCA) *NativeStepCAIssuer {
	t.Helper()
	url, root := ca.start(t)
	pw := writePasswordFile(t, t.TempDir(), "pki-agent-alt-backend-jwk", string(ca.password), 0o400)
	return &NativeStepCAIssuer{
		CAURL:        url,
		RootFile:     root,
		Provisioner:  ca.provisioner,
		PasswordFile: pw,
		Timeout:      10 * time.Second,
	}
}

func httptestClient(t *testing.T, rootFile string) *http.Client {
	t.Helper()
	data, err := os.ReadFile(rootFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		t.Fatal("root pem")
	}
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool},
			DialContext:     (&net.Dialer{Timeout: time.Second}).DialContext,
		},
	}
}

func mustCSRPEM(t *testing.T, cn string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: cn},
		DNSNames: []string{cn},
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}
