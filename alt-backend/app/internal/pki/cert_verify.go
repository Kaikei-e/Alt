package pki

import (
	"bytes"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"strings"

	"go.step.sm/crypto/pemutil"
	"go.step.sm/crypto/x509util"
)

// classifyCAStatus maps HTTP error status codes into CA error categories.
func classifyCAStatus(code int) error {
	if code >= 500 {
		return fmt.Errorf("%w (status %d)", ErrCAUnavailable, code)
	}
	return fmt.Errorf("%w (status %d)", ErrCARejected, code)
}

// parseCertPEM decodes and parses the first PEM-encoded x509 certificate.
func parseCertPEM(s string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		return nil, fmt.Errorf("pki: no certificate pem")
	}
	return x509.ParseCertificate(block.Bytes)
}

// certMatchesSANs asserts the certificate contains exactly the requested Subject Alternative Names.
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

// sameFoldedSet checks case-folded string equality across sets.
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

// sameIPSet checks IP equality across sets regardless of order.
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

// sameURISet checks URL string equality across sets regardless of order.
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

// publicKeyMatches ensures the certificate's public key corresponds to the private key signer.
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

// requireDualEKU enforces both client and server authentication key usages on issued leaves.
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

// encodeIssued serializes the issued certificate chain and private key into usable PEM blocks.
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
