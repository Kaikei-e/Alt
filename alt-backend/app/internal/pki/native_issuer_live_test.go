package pki

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	liveCAImage     = "smallstep/step-ca:0.30.2"
	liveCAContainer = "alt-pki-native-itest"
	liveCANetwork   = "alt-pki-native-itest-net"
	liveCAHostPort  = "19000"
	liveCAPassword  = "itest-only"
	liveJWKPassword = "itest-jwk-alt-backend"
	liveProvisioner = "pki-agent-alt-backend"
	liveSubject     = "alt-backend"
)

// TestNativeStepCAIssuer_DisposableLiveCA is the Wave 4 Go-cohort live gate.
// Default `go test ./...` skips it so the suite never talks to Alt's compose
// CA (compose/pki.yaml) or creates real provisioner secrets.
//
// Isolated run (60s, always cleanup, never alt-step-ca-1 / project alt):
//
//	PKI_NATIVE_LIVE_CA=1 go test ./internal/pki/ -count=1 -timeout 60s \
//	  -run TestNativeStepCAIssuer_DisposableLiveCA
//
// Container (documented command + WITH_CA_URL so the OTT audience matches the
// host-mapped port; CA still listens on :9000 inside the container):
//
//	docker network create alt-pki-native-itest-net
//	docker run --rm -d --name alt-pki-native-itest \
//	  --network alt-pki-native-itest-net \
//	  -p 127.0.0.1:19000:9000 \
//	  -e DOCKER_STEPCA_INIT_NAME=alt-itest \
//	  -e DOCKER_STEPCA_INIT_DNS_NAMES=localhost,127.0.0.1 \
//	  -e DOCKER_STEPCA_INIT_PASSWORD=itest-only \
//	  -e DOCKER_STEPCA_INIT_WITH_CA_URL=https://127.0.0.1:19000 \
//	  smallstep/step-ca:0.30.2
func TestNativeStepCAIssuer_DisposableLiveCA(t *testing.T) {
	if os.Getenv("PKI_NATIVE_LIVE_CA") != "1" {
		t.Skip("set PKI_NATIVE_LIVE_CA=1 to run isolated disposable step-ca; never talks to Alt compose CA")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker is required for the live gate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	tmp, err := os.MkdirTemp("", "alt-pki-native-itest-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	t.Cleanup(func() {
		_, _ = docker(context.Background(), "rm", "-f", liveCAContainer)
		_, _ = docker(context.Background(), "network", "rm", liveCANetwork)
	})
	_, _ = docker(context.Background(), "rm", "-f", liveCAContainer)
	_, _ = docker(context.Background(), "network", "rm", liveCANetwork)

	if out, err := docker(ctx, "network", "create", liveCANetwork); err != nil {
		t.Fatalf("network create: %v (%s)", err, out)
	}
	runArgs := []string{
		"run", "--rm", "-d",
		"--name", liveCAContainer,
		"--network", liveCANetwork,
		"-p", "127.0.0.1:" + liveCAHostPort + ":9000",
		"-e", "DOCKER_STEPCA_INIT_NAME=alt-itest",
		"-e", "DOCKER_STEPCA_INIT_DNS_NAMES=localhost,127.0.0.1",
		"-e", "DOCKER_STEPCA_INIT_PASSWORD=" + liveCAPassword,
		"-e", "DOCKER_STEPCA_INIT_WITH_CA_URL=https://127.0.0.1:" + liveCAHostPort,
		liveCAImage,
	}
	if out, err := docker(ctx, runArgs...); err != nil {
		t.Fatalf("docker run: %v (%s)", err, out)
	}

	rootFile := filepath.Join(tmp, "root_ca.crt")
	waitLiveCARoot(t, ctx, rootFile)
	addLiveJWKProvisioner(t, ctx)
	passwordFile := writePasswordFile(t, tmp, "pki-agent-alt-backend-jwk", liveJWKPassword, 0o400)

	caURL := "https://127.0.0.1:" + liveCAHostPort
	iss := &NativeStepCAIssuer{
		CAURL:        caURL,
		RootFile:     rootFile,
		Provisioner:  liveProvisioner,
		PasswordFile: passwordFile,
		Timeout:      10 * time.Second,
	}

	certPEM, keyPEM, err := iss.Issue(ctx, liveSubject, []string{liveSubject})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	leaf1 := mustLiveLeaf(t, certPEM, keyPEM, rootFile)

	rekeyCert, rekeyKey, err := iss.Rekey(ctx, certPEM, keyPEM, liveSubject, []string{liveSubject})
	if err != nil {
		t.Fatalf("rekey: %v", err)
	}
	leaf2 := mustLiveLeaf(t, rekeyCert, rekeyKey, rootFile)
	if bytes.Equal(leaf1.Raw, leaf2.Raw) {
		t.Fatal("rekey returned the same certificate")
	}
	if publicKeyEqual(leaf1, leaf2) {
		t.Fatal("rekey reused the leaf public key")
	}

	dir := t.TempDir()
	files := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	mgr := &Manager{
		Cfg: Config{
			Subject:         liveSubject,
			SANs:            []string{liveSubject},
			Provisioner:     liveProvisioner,
			RenewAtFraction: 0.66,
		},
		Issuer: iss,
		Files:  files,
	}
	if err := files.Write(ctx, rekeyCert, rekeyKey); err != nil {
		t.Fatal(err)
	}

	near := leaf2.NotBefore.Add(leaf2.NotAfter.Sub(leaf2.NotBefore) * 3 / 4)
	mgr.Now = func() time.Time { return near }
	state, err := mgr.Tick(ctx)
	if err != nil {
		t.Fatalf("near-expiry tick (rekey): %v", err)
	}
	if state != StateFresh && state != StateNearExpiry {
		t.Fatalf("near-expiry state=%s", state)
	}
	afterRekey, err := os.ReadFile(files.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	leaf3 := mustLiveLeaf(t, afterRekey, mustRead(t, files.KeyPath), rootFile)
	if bytes.Equal(leaf2.Raw, leaf3.Raw) {
		t.Fatal("manager rekey did not replace the leaf")
	}

	expired := leaf3.NotAfter.Add(time.Minute)
	mgr.Now = func() time.Time { return expired }
	if _, err := mgr.Tick(ctx); err != nil {
		t.Fatalf("expired tick (re-enroll via /sign): %v", err)
	}
	afterExpire, err := os.ReadFile(files.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = mustLiveLeaf(t, afterExpire, mustRead(t, files.KeyPath), rootFile)
	if bytes.Equal(leaf3.Raw, afterExpire) {
		t.Fatal("expired re-enroll did not replace the leaf")
	}

	if err := os.Remove(files.CertPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(files.KeyPath); err != nil {
		t.Fatal(err)
	}
	mgr.Now = time.Now
	if _, err := mgr.Tick(ctx); err != nil {
		t.Fatalf("missing tick (re-enroll via /sign): %v", err)
	}
	afterMissing, err := os.ReadFile(files.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = mustLiveLeaf(t, afterMissing, mustRead(t, files.KeyPath), rootFile)
}

func waitLiveCARoot(t *testing.T, ctx context.Context, dest string) {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(20 * time.Second)
	}
	var last string
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait for CA root: %v (last=%s)", err, last)
		}
		out, err := docker(ctx, "exec", liveCAContainer, "test", "-f", "/home/step/certs/root_ca.crt")
		if err == nil {
			if cp, err := docker(ctx, "cp", liveCAContainer+":/home/step/certs/root_ca.crt", dest); err != nil {
				t.Fatalf("docker cp root: %v (%s)", err, cp)
			}
			return
		}
		last = strings.TrimSpace(out)
		time.Sleep(200 * time.Millisecond)
	}
	logs, _ := docker(ctx, "logs", liveCAContainer)
	t.Fatalf("CA root did not appear: %s\nlogs:\n%s", last, logs)
}

func addLiveJWKProvisioner(t *testing.T, ctx context.Context) {
	t.Helper()
	script := fmt.Sprintf(`set -euo pipefail
printf '%%s\n' %q > /tmp/pki-agent-alt-backend-jwk
chmod 400 /tmp/pki-agent-alt-backend-jwk
step ca provisioner add %q --type JWK --create \
  --password-file /tmp/pki-agent-alt-backend-jwk \
  --ca-config /home/step/config/ca.json
kill -HUP 1
`, liveJWKPassword, liveProvisioner)
	out, err := docker(ctx, "exec", liveCAContainer, "sh", "-c", script)
	if err != nil {
		t.Fatalf("provisioner add: %v (%s)", err, out)
	}
	cfg, err := docker(ctx, "exec", liveCAContainer, "grep", "-F", liveProvisioner, "/home/step/config/ca.json")
	if err != nil || !strings.Contains(cfg, liveProvisioner) {
		t.Fatalf("provisioner not in ca.json: %v (%s)", err, cfg)
	}
	deadline := time.Now().Add(8 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			t.Fatalf("wait for provisioner API: %v (last=%s)", err, last)
		}
		out, err = docker(ctx, "exec", liveCAContainer, "step", "ca", "provisioner", "list",
			"--ca-url", "https://localhost:9000", "--root", "/home/step/certs/root_ca.crt")
		last = strings.TrimSpace(out)
		if err == nil && strings.Contains(out, liveProvisioner) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("provisioner %s not served after SIGHUP: %s", liveProvisioner, last)
}

func mustLiveLeaf(t *testing.T, certPEM, keyPEM []byte, rootFile string) *x509.Certificate {
	t.Helper()
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("key pair: %v", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("no cert pem")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if leaf.Subject.CommonName != liveSubject {
		t.Fatalf("cn=%q", leaf.Subject.CommonName)
	}
	if err := certMatchesSANs(leaf, []string{liveSubject}); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	pemBytes, err := os.ReadFile(rootFile)
	if err != nil {
		t.Fatal(err)
	}
	if !roots.AppendCertsFromPEM(pemBytes) {
		t.Fatal("pinned root did not parse")
	}
	intermediates := x509.NewCertPool()
	rest := certPEM
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil || bytes.Equal(c.Raw, leaf.Raw) {
			continue
		}
		intermediates.AddCert(c)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		DNSName:       liveSubject,
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Fatalf("chain verify: %v", err)
	}
	return leaf
}

func publicKeyEqual(a, b *x509.Certificate) bool {
	ak, ok := a.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return false
	}
	bk, ok := b.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return false
	}
	return ak.Equal(bk)
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
