package pki

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.step.sm/crypto/jose"
)

func TestNativeStepCAIssuer_Issue_Success(t *testing.T) {
	password := []byte("subject-scoped-jwk-password")
	ca := newFakeStepCA(t, "pki-agent-alt-backend", password)
	iss := newNativeIssuer(t, ca)

	certPEM, keyPEM, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
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
	if claims.Issuer != "pki-agent-alt-backend" {
		t.Fatalf("iss=%q", claims.Issuer)
	}
	if claims.Subject != "alt-backend" {
		t.Fatalf("sub=%q", claims.Subject)
	}
	if !audienceHasSign(claims.Audience) {
		t.Fatalf("aud=%v", claims.Audience)
	}
	if len(claims.SANs) == 0 || claims.SANs[0] != "alt-backend" {
		t.Fatalf("sans=%v", claims.SANs)
	}

	block, _ := pem.Decode(certPEM)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != "alt-backend" {
		t.Fatalf("cn=%q", leaf.Subject.CommonName)
	}
}

func TestNativeStepCAIssuer_MintsDistinctOTTs(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-distinct"))
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); err != nil {
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
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-reuse"))
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); err != nil {
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
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-exp"))
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
		"sub":  "alt-backend",
		"aud":  iss.CAURL + "/1.0/sign",
		"sans": []string{"alt-backend"},
		"jti":  "expired-jti",
		"nbf":  now.Unix(),
		"iat":  now.Unix(),
		"exp":  now.Add(time.Minute).Unix(),
	}).CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	csrPEM := mustCSRPEM(t, "alt-backend")
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
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-root"))
	iss := newNativeIssuer(t, ca)
	other, _ := generateTestCA(t)
	wrongRoot := filepath.Join(t.TempDir(), "wrong.pem")
	if err := os.WriteFile(wrongRoot, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: other.Raw}), 0o444); err != nil {
		t.Fatal(err)
	}
	iss.RootFile = wrongRoot
	_, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
	if err == nil {
		t.Fatal("expected wrong-root failure")
	}
	if strings.Contains(strings.ToLower(err.Error()), "insecure") {
		t.Fatalf("must not skip TLS verify: %v", err)
	}
}

func TestNativeStepCAIssuer_WrongSubjectRejected(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-sub"))
	ca.mutateLeaf = func(c *x509.Certificate) {
		c.Subject.CommonName = "evil"
		c.DNSNames = []string{"evil"}
	}
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); err == nil {
		t.Fatal("expected subject mismatch")
	}
}

func TestNativeStepCAIssuer_WrongSANRejected(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-san"))
	ca.mutateLeaf = func(c *x509.Certificate) {
		c.DNSNames = []string{"not-the-requested-san"}
	}
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); err == nil {
		t.Fatal("expected SAN mismatch")
	}
}

func TestNativeStepCAIssuer_MalformedResponse(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-malformed"))
	ca.malformedSign = true
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); err == nil {
		t.Fatal("expected malformed response error")
	}
}

func TestNativeStepCAIssuer_Timeout(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-timeout"))
	ca.signDelay = 2 * time.Second
	iss := newNativeIssuer(t, ca)
	if _, err := iss.credentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	iss.Timeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	_, _, err := iss.Issue(ctx, "alt-backend", []string{"alt-backend"})
	if err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("timeout did not bound the call: %v", time.Since(start))
	}
}

func TestNativeStepCAIssuer_Canceled(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-cancel"))
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
		_, _, err := iss.Issue(ctx, "alt-backend", []string{"alt-backend"})
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
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-files"))
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
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("super-secret-jwk-password"))
	iss := newNativeIssuer(t, ca)
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); err != nil {
		t.Fatal(err)
	}
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
	iss := &NativeStepCAIssuer{Provisioner: "pki-agent", PasswordFile: "/run/secrets/pki-agent-alt-backend-jwk"}
	if _, _, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"}); !errors.Is(err, ErrSharedProvisioner) {
		t.Fatalf("got %v", err)
	}
}

func TestNativeStepCAIssuer_RekeyUsesClientCert(t *testing.T) {
	ca := newFakeStepCA(t, "pki-agent-alt-backend", []byte("pw-rekey"))
	iss := newNativeIssuer(t, ca)
	certPEM, keyPEM, err := iss.Issue(context.Background(), "alt-backend", []string{"alt-backend"})
	if err != nil {
		t.Fatal(err)
	}
	newCert, newKey, err := iss.Rekey(context.Background(), certPEM, keyPEM, "alt-backend", []string{"alt-backend"})
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
