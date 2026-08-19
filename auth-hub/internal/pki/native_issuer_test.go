package pki

import (
	"bytes"
	"context"
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
	endlessProvisioners bool
	wrongLeafKey        bool
	redirect            bool
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
		MinVersion: tls.VersionTLS13,
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
	if f.redirect {
		http.Redirect(w, r, "https://evil.example/sign", http.StatusFound)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/health":
		if f.holdHealth != nil {
			<-f.holdHealth
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/provisioners":
		f.writeProvisioners(w, r)
	case r.Method == http.MethodPost && (r.URL.Path == "/sign" || r.URL.Path == "/1.0/sign"):
		f.handleSign(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/rekey":
		f.handleRekey(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/renew":
		http.Error(w, "renew requires a valid client certificate; expired leaves must re-enroll", http.StatusUnauthorized)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeStepCA) writeProvisioners(w http.ResponseWriter, r *http.Request) {
	if f.endlessProvisioners {
		cursor := r.URL.Query().Get("cursor")
		next := "page-2"
		if cursor != "" {
			next = cursor + "x"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"provisioners":[],"nextCursor":` + jsonString(next) + `}`))
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
	pub := csr.PublicKey
	if f.wrongLeafKey {
		other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			f.t.Fatal(err)
		}
		pub = &other.PublicKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, f.caCert, pub, f.caKey)
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
	pw := writePasswordFile(t, t.TempDir(), "pki-agent-auth-hub-jwk", string(ca.password), 0o400)
	return &NativeStepCAIssuer{
		CAURL:        url,
		RootFile:     root,
		Provisioner:  ca.provisioner,
		PasswordFile: pw,
		Timeout:      10 * time.Second,
	}
}

func TestNativeStepCAIssuer_Issue_Success(t *testing.T) {
	password := []byte("subject-scoped-jwk-password")
	ca := newFakeStepCA(t, "pki-agent-auth-hub", password)
	iss := newNativeIssuer(t, ca)

	certPEM, keyPEM, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"})
	if err != nil {
		ca.mu.Lock()
		seen := append([]recordedReq(nil), ca.seen...)
		ca.mu.Unlock()
		t.Fatalf("issue: %v seen=%s", err, summarizeSeen(seen))
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Fatal("empty pem")
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("key pair: %v", err)
	}

	got := ca.lastSign()
	if got.Method != http.MethodPost {
		t.Fatalf("method=%s", got.Method)
	}
	if got.Path != "/sign" {
		t.Fatalf("path=%s want /sign", got.Path)
	}
	var body struct {
		CSR string `json:"csr"`
		OTT string `json:"ott"`
	}
	if err := json.Unmarshal(got.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.OTT == "" || body.CSR == "" {
		t.Fatal("missing ott or csr")
	}
	claims, err := ca.verifyOTTForTest(body.OTT)
	if err != nil {
		t.Fatalf("ott claims: %v", err)
	}
	if claims.Issuer != "pki-agent-auth-hub" {
		t.Fatalf("iss=%q", claims.Issuer)
	}
	if claims.Subject != "auth-hub" {
		t.Fatalf("sub=%q", claims.Subject)
	}
	if !audienceHasSign(claims.Audience) {
		t.Fatalf("aud=%v", claims.Audience)
	}
	if len(claims.SANs) == 0 || claims.SANs[0] != "auth-hub" {
		t.Fatalf("sans=%v", claims.SANs)
	}

	block, _ := pem.Decode(certPEM)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != "auth-hub" {
		t.Fatalf("cn=%q", leaf.Subject.CommonName)
	}
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

func TestNativeStepCAIssuer_MintsDistinctOTTs(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-distinct"))
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); err != nil {
		t.Fatal(err)
	}
	var jtis []string
	ca.mu.Lock()
	for _, rec := range ca.seen {
		if rec.Path != "/sign" {
			continue
		}
		var body struct {
			OTT string `json:"ott"`
		}
		_ = json.Unmarshal(rec.Body, &body)
		claims, err := ca.verifyOTTForTest(body.OTT)
		if err != nil {
			t.Fatal(err)
		}
		jtis = append(jtis, claims.ID)
	}
	ca.mu.Unlock()
	if len(jtis) != 2 || jtis[0] == "" || jtis[0] == jtis[1] {
		t.Fatalf("jtis=%v", jtis)
	}
}

func TestNativeStepCAIssuer_ReusedOTTRejectedByCA(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-reuse"))
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); err != nil {
		t.Fatal(err)
	}
	first := ca.lastSign()
	req, err := http.NewRequest(http.MethodPost, iss.CAURL+"/sign", bytes.NewReader(first.Body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := httptestClient(t, iss.RootFile)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 400 {
		t.Fatalf("reused ott status=%d", resp.StatusCode)
	}
}

func TestNativeStepCAIssuer_ExpiredOTTRejectedByCA(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-exp"))
	iss := newNativeIssuer(t, ca)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: ca.jwk.Key}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]interface{}{"kid": ca.jwk.KeyID},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Hour)
	raw, err := jose.Signed(signer).Claims(map[string]any{
		"iss":  ca.provisioner,
		"sub":  "auth-hub",
		"aud":  iss.CAURL + "/1.0/sign",
		"sans": []string{"auth-hub"},
		"jti":  "expired-jti",
		"nbf":  now.Unix(),
		"iat":  now.Unix(),
		"exp":  now.Add(time.Minute).Unix(),
	}).CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	csrPEM := mustCSRPEM(t, "auth-hub")
	body, _ := json.Marshal(map[string]string{"csr": csrPEM, "ott": raw})
	req, err := http.NewRequest(http.MethodPost, iss.CAURL+"/sign", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httptestClient(t, iss.RootFile).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 400 {
		t.Fatalf("expired ott status=%d", resp.StatusCode)
	}
}

func TestNativeStepCAIssuer_WrongCARoot(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-root"))
	iss := newNativeIssuer(t, ca)
	other, _ := generateTestCA(t)
	wrongRoot := filepath.Join(t.TempDir(), "wrong.pem")
	if err := os.WriteFile(wrongRoot, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: other.Raw}), 0o444); err != nil {
		t.Fatal(err)
	}
	iss.RootFile = wrongRoot
	_, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"})
	if err == nil {
		t.Fatal("expected wrong-root failure")
	}
	if strings.Contains(strings.ToLower(err.Error()), "insecure") {
		t.Fatalf("must not skip TLS verify: %v", err)
	}
}

func TestNativeStepCAIssuer_WrongSubjectRejected(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-sub"))
	ca.mutateLeaf = func(c *x509.Certificate) {
		c.Subject.CommonName = "evil"
		c.DNSNames = []string{"evil"}
	}
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); err == nil {
		t.Fatal("expected subject mismatch")
	}
}

func TestNativeStepCAIssuer_WrongSANRejected(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-san"))
	ca.mutateLeaf = func(c *x509.Certificate) {
		c.DNSNames = []string{"not-the-requested-san"}
	}
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); err == nil {
		t.Fatal("expected SAN mismatch")
	}
}

func TestNativeStepCAIssuer_MalformedResponse(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-malformed"))
	ca.malformedSign = true
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); err == nil {
		t.Fatal("expected malformed response error")
	}
}

func TestNativeStepCAIssuer_Timeout(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-timeout"))
	ca.signDelay = 2 * time.Second
	iss := newNativeIssuer(t, ca)
	iss.Timeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	_, _, err := iss.Issue(ctx, "auth-hub", []string{"auth-hub"})
	if err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("timeout did not bound the call: %v", time.Since(start))
	}
}

func TestNativeStepCAIssuer_Canceled(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-cancel"))
	ca.blockSign = make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-ca.blockSign:
		default:
			close(ca.blockSign)
		}
	})
	iss := newNativeIssuer(t, ca)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := iss.Issue(ctx, "auth-hub", []string{"auth-hub"})
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "cancel") {
			t.Fatalf("got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Issue did not observe cancel")
	}
}

func TestNativeStepCAIssuer_PasswordFileErrors(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-files"))
	url, root := ca.start(t)
	tests := []struct {
		name string
		file func(t *testing.T) string
	}{
		{
			name: "missing",
			file: func(t *testing.T) string { return filepath.Join(t.TempDir(), "nope") },
		},
		{
			name: "empty",
			file: func(t *testing.T) string {
				return writePasswordFile(t, t.TempDir(), "empty", "", 0o400)
			},
		},
		{
			name: "directory",
			file: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name: "world-writable",
			file: func(t *testing.T) string {
				return writePasswordFile(t, t.TempDir(), "open", "secret", 0o666)
			},
		},
		{
			name: "group-writable",
			file: func(t *testing.T) string {
				return writePasswordFile(t, t.TempDir(), "group", "secret", 0o660)
			},
		},
		{
			name: "too-large",
			file: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "huge")
				if err := os.WriteFile(path, bytes.Repeat([]byte("a"), maxPasswordBytes+1), 0o400); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
		{
			name: "symlink",
			file: func(t *testing.T) string {
				dir := t.TempDir()
				target := writePasswordFile(t, dir, "real", "secret", 0o400)
				link := filepath.Join(dir, "link")
				if err := os.Symlink(target, link); err != nil {
					t.Fatal(err)
				}
				return link
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.file(t)
			subject := strings.TrimPrefix(ca.provisioner, "pki-agent-")
			iss := &NativeStepCAIssuer{
				CAURL: url, RootFile: root,
				Provisioner:  ca.provisioner,
				PasswordFile: path,
				Timeout:      time.Second,
			}
			_, _, err := iss.Issue(context.Background(), subject, []string{subject})
			if err == nil {
				t.Fatal("expected password file error")
			}
			assertNoPasswordFileInError(t, err, path, "secret")
			switch tt.name {
			case "empty":
				if !errors.Is(err, ErrPasswordEmpty) {
					t.Fatalf("got %v, want ErrPasswordEmpty", err)
				}
			case "too-large":
				if !errors.Is(err, ErrPasswordTooLarge) {
					t.Fatalf("got %v, want ErrPasswordTooLarge", err)
				}
			default:
				if !errors.Is(err, ErrPasswordUnreadable) {
					t.Fatalf("got %v, want ErrPasswordUnreadable", err)
				}
			}
		})
	}
}

func TestNativeStepCAIssuer_DoesNotLogSecrets(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("super-secret-jwk-password"))
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); err != nil {
		t.Fatal(err)
	}
	// Error paths must also keep the password out of the error chain.
	missing := filepath.Join(t.TempDir(), "missing")
	iss.PasswordFile = missing
	iss.cred = nil
	_, _, err := iss.Issue(context.Background(), strings.TrimPrefix(iss.Provisioner, "pki-agent-"), []string{strings.TrimPrefix(iss.Provisioner, "pki-agent-")})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPasswordUnreadable) {
		t.Fatalf("got %v, want ErrPasswordUnreadable", err)
	}
	assertNoPasswordFileInError(t, err, missing, "super-secret-jwk-password", "/run/secrets/")
}

func TestNativeStepCAIssuer_RejectsSharedProvisioner(t *testing.T) {
	iss := &NativeStepCAIssuer{Provisioner: "pki-agent", PasswordFile: "/run/secrets/pki-agent-auth-hub-jwk"}
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); !errors.Is(err, ErrSharedProvisioner) {
		t.Fatalf("got %v", err)
	}
}

func TestNativeStepCAIssuer_RejectsHTTP(t *testing.T) {
	iss := &NativeStepCAIssuer{
		CAURL:        "http://step-ca:9000",
		Provisioner:  "pki-agent-auth-hub",
		PasswordFile: "/run/secrets/pki-agent-auth-hub-jwk",
	}
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); !errors.Is(err, ErrNotHTTPS) {
		t.Fatalf("got %v", err)
	}
}

func TestNativeStepCAIssuer_RejectsRedirect(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-redirect"))
	ca.redirect = true
	iss := newNativeIssuer(t, ca)
	_, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"})
	if !errors.Is(err, ErrRedirect) {
		t.Fatalf("got %v", err)
	}
}

func TestNativeStepCAIssuer_PasswordTooLarge(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-cap"))
	url, root := ca.start(t)
	path := filepath.Join(t.TempDir(), "huge")
	if err := os.WriteFile(path, bytes.Repeat([]byte("a"), maxPasswordBytes+1), 0o400); err != nil {
		t.Fatal(err)
	}
	iss := &NativeStepCAIssuer{
		CAURL: url, RootFile: root,
		Provisioner:  "pki-agent-auth-hub",
		PasswordFile: path,
		Timeout:      time.Second,
	}
	_, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"})
	if !errors.Is(err, ErrPasswordTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func TestNativeStepCAIssuer_AcceptsWorldReadablePassword(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("docker-secret"))
	url, root := ca.start(t)
	pw := writePasswordFile(t, t.TempDir(), "pki-agent-auth-hub-jwk", string(ca.password), 0o444)
	iss := &NativeStepCAIssuer{
		CAURL: url, RootFile: root,
		Provisioner:  ca.provisioner,
		PasswordFile: pw,
		Timeout:      10 * time.Second,
	}
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); err != nil {
		t.Fatal(err)
	}
}

func TestNativeStepCAIssuer_ProvisionerPageCap(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-pages"))
	ca.endlessProvisioners = true
	iss := newNativeIssuer(t, ca)
	_, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"})
	if !errors.Is(err, ErrProvisionerPageLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestNativeStepCAIssuer_ResponseTooLarge(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-huge-body"))
	ca.signStatus = http.StatusCreated
	ca.signBody = bytes.Repeat([]byte("a"), int(maxResponseBytes)+8)
	iss := newNativeIssuer(t, ca)
	_, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"})
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func TestNativeStepCAIssuer_CAErrorStatusOnly(t *testing.T) {
	const jwt = "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.payload.sig"
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-leak"))
	ca.signStatus = http.StatusUnauthorized
	ca.signBody = []byte(`{"status":401,"message":"ott invalid ` + jwt + `"}`)
	iss := newNativeIssuer(t, ca)
	_, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"})
	if err == nil {
		t.Fatal("expected CA rejection")
	}
	if !errors.Is(err, ErrCARejected) {
		t.Fatalf("sentinel: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "401") {
		t.Fatalf("status missing: %v", err)
	}
	if strings.Contains(msg, jwt) || strings.Contains(msg, "ott invalid") {
		t.Fatalf("CA body leaked: %v", err)
	}
}

func TestNativeStepCAIssuer_ExtraDNSSANRejected(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-extra-san"))
	ca.mutateLeaf = func(c *x509.Certificate) {
		c.DNSNames = append(append([]string{}, c.DNSNames...), "extra.example")
	}
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); err == nil {
		t.Fatal("expected extra DNS SAN rejection")
	}
}

func TestNativeStepCAIssuer_TypedSANSet(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-typed-san"))
	iss := newNativeIssuer(t, ca)
	sans := []string{"auth-hub", "127.0.0.1", "https://alt.example/svc"}
	certPEM, _, err := iss.Issue(context.Background(), "auth-hub", sans)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(certPEM)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != "auth-hub" {
		t.Fatalf("dns=%v", leaf.DNSNames)
	}
	if len(leaf.IPAddresses) != 1 || leaf.IPAddresses[0].String() != "127.0.0.1" {
		t.Fatalf("ips=%v", leaf.IPAddresses)
	}
	if len(leaf.URIs) != 1 || leaf.URIs[0].String() != "https://alt.example/svc" {
		t.Fatalf("uris=%v", leaf.URIs)
	}
}

func TestNativeStepCAIssuer_WrongLeafPublicKeyRejected(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-wrong-key"))
	ca.wrongLeafKey = true
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"}); err == nil {
		t.Fatal("expected leaf/key mismatch")
	}
}

func TestNativeStepCAIssuer_RekeyUsesClientCert(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-auth-hub", []byte("pw-rekey"))
	iss := newNativeIssuer(t, ca)
	certPEM, keyPEM, err := iss.Issue(context.Background(), "auth-hub", []string{"auth-hub"})
	if err != nil {
		t.Fatal(err)
	}
	newCert, newKey, err := iss.Rekey(context.Background(), certPEM, keyPEM, "auth-hub", []string{"auth-hub"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(newCert, newKey); err != nil {
		t.Fatal(err)
	}
	got := ca.lastRekey()
	if got.Method != http.MethodPost || got.Path != "/rekey" {
		t.Fatalf("rekey request %s %s", got.Method, got.Path)
	}
	if bytes.Equal(certPEM, newCert) {
		t.Fatal("rekey returned the same cert bytes")
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
