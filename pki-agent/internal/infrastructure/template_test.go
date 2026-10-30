package infrastructure_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

type templateTestCase struct {
	Name      string   `json:"name"`
	Template  string   `json:"template"`
	TokenSub  string   `json:"token_sub"`
	TokenSANs []string `json:"token_sans"`
	CSRCN     string   `json:"csr_cn"`
	CSRDNS    []string `json:"csr_dns"`
	CSRIPs    []string `json:"csr_ips"`
	CSRURIs   []string `json:"csr_uris"`
	CSREmails []string `json:"csr_emails"`
	WantErr   bool     `json:"-"`
}

func evaluateTemplateCase(tc templateTestCase) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	var ips []net.IP
	for _, ipStr := range tc.CSRIPs {
		if ip := net.ParseIP(ipStr); ip != nil {
			ips = append(ips, ip)
		}
	}
	var uris []*url.URL
	for _, uriStr := range tc.CSRURIs {
		if u, err := url.Parse(uriStr); err == nil {
			uris = append(uris, u)
		}
	}
	csrTemplate := &x509.CertificateRequest{
		Subject:        pkix.Name{CommonName: tc.CSRCN},
		DNSNames:       tc.CSRDNS,
		IPAddresses:    ips,
		URIs:           uris,
		EmailAddresses: tc.CSREmails,
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, priv)
	if err != nil {
		return err
	}
	cr, err := x509.ParseCertificateRequest(csrBytes)
	if err != nil {
		return err
	}

	tmpl, err := template.New("x509").Funcs(template.FuncMap{
		"fail": func(msg string) (string, error) {
			return "", errors.New(msg)
		},
	}).Parse(tc.Template)
	if err != nil {
		return err
	}

	data := map[string]any{
		"Token": map[string]any{
			"sub":  tc.TokenSub,
			"sans": tc.TokenSANs,
		},
		"Insecure": map[string]any{
			"CR": cr,
		},
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	var rendered map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rendered); err != nil {
		return fmt.Errorf("rendered template is not valid JSON: %w", err)
	}
	return nil
}

func findRepoRoot() (string, error) {
	candidates := []string{
		"../../..",
		"../..",
		"..",
		".",
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, "pki-agent", "scripts", "bootstrap-pki-provisioner.sh")); err == nil {
			return abs, nil
		}
	}
	return "", fmt.Errorf("cannot locate repository containing pki-agent/scripts/bootstrap-pki-provisioner.sh")
}

func TestFindRepoRootPortableWithoutCompose(t *testing.T) {
	root := t.TempDir()
	scriptsDir := filepath.Join(root, "pki-agent", "scripts")
	if err := os.MkdirAll(scriptsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "bootstrap-pki-provisioner.sh"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	packageDir := filepath.Join(root, "pki-agent", "internal", "infrastructure")
	if err := os.MkdirAll(packageDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(packageDir)
	got, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("repository root = %q, want sparse checkout root %q", got, root)
	}
}

func extractHeredocTemplates(t *testing.T) (subjectTpl, localhostTpl string) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("cannot find repo root: %v", err)
	}
	scriptPath := filepath.Join(repoRoot, "pki-agent", "scripts", "bootstrap-pki-provisioner.sh")
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("cannot read bootstrap script %s: %v", scriptPath, err)
	}
	content := string(data)

	// Extract localhost template
	locMarker := "if [ \"$subject\" = \"localhost\" ]; then"
	locIdx := strings.Index(content, locMarker)
	if locIdx == -1 {
		t.Fatalf("failed to find localhost marker %q in script", locMarker)
	}
	catMarker := "cat <<EOF\n"
	locCatIdx := strings.Index(content[locIdx:], catMarker)
	if locCatIdx == -1 {
		t.Fatalf("failed to find localhost heredoc start in script")
	}
	locStart := locIdx + locCatIdx + len(catMarker)
	locEnd := strings.Index(content[locStart:], "\nEOF")
	if locEnd == -1 {
		t.Fatalf("failed to find localhost heredoc end in script")
	}
	rawLocalhost := content[locStart : locStart+locEnd]
	localhostTpl = strings.ReplaceAll(rawLocalhost, `\$has_subject`, "$has_subject")

	// Extract subject template
	elseMarker := "else\n    cat <<EOF\n"
	elseIdx := strings.Index(content[locStart+locEnd:], elseMarker)
	if elseIdx == -1 {
		t.Fatalf("failed to find subject heredoc start in script")
	}
	subStart := locStart + locEnd + elseIdx + len(elseMarker)
	subEnd := strings.Index(content[subStart:], "\nEOF")
	if subEnd == -1 {
		t.Fatalf("failed to find subject heredoc end in script")
	}
	rawSubject := content[subStart : subStart+subEnd]
	subjectTpl = strings.ReplaceAll(rawSubject, `\$has_subject`, "$has_subject")
	subjectTpl = strings.ReplaceAll(subjectTpl, "${subject}", "alt-backend")

	return subjectTpl, localhostTpl
}

func executeTemplateCases(t *testing.T, cases []templateTestCase) {
	t.Helper()
	for _, tc := range cases {
		err := evaluateTemplateCase(tc)
		hasErr := err != nil
		if hasErr != tc.WantErr {
			t.Errorf("[%s] error mismatch: got error=%v (hasErr=%v), wantErr=%v", tc.Name, err, hasErr, tc.WantErr)
		}
	}
}

func TestX509TemplateConstraints(t *testing.T) {
	subjectTpl, _ := extractHeredocTemplates(t)

	tests := []templateTestCase{
		{
			Name:      "subject_valid_with_localhost",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend", "localhost"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend", "localhost"},
			WantErr:   false,
		},
		{
			Name:      "subject_valid_subject_only",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend"},
			WantErr:   false,
		},
		{
			Name:      "subject_foreign_ott_sub",
			Template:  subjectTpl,
			TokenSub:  "alt-harvester",
			TokenSANs: []string{"alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend"},
			WantErr:   true,
		},
		{
			Name:      "subject_foreign_ott_sans",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend", "alt-harvester"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend"},
			WantErr:   true,
		},
		{
			Name:      "subject_foreign_csr_cn",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend"},
			CSRCN:     "alt-harvester",
			CSRDNS:    []string{"alt-backend"},
			WantErr:   true,
		},
		{
			Name:      "subject_foreign_csr_dns",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend", "alt-harvester"},
			WantErr:   true,
		},
		{
			Name:      "subject_missing_own_dns",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"localhost"},
			WantErr:   true,
		},
		{
			Name:      "subject_csr_ip_sans",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend"},
			CSRIPs:    []string{"127.0.0.1"},
			WantErr:   true,
		},
		{
			Name:      "subject_csr_uri_sans",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend"},
			CSRURIs:   []string{"spiffe://alt/backend"},
			WantErr:   true,
		},
		{
			Name:      "subject_csr_email_sans",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend"},
			CSREmails: []string{"admin@alt.internal"},
			WantErr:   true,
		},
		{
			Name:      "subject_duplicate_ott_sans",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend", "alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend"},
			WantErr:   true,
		},
		{
			Name:      "subject_duplicate_csr_dns",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend", "alt-backend"},
			WantErr:   true,
		},
		{
			Name:      "subject_too_many_ott_sans",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend", "localhost", "alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend"},
			WantErr:   true,
		},
		{
			Name:      "subject_too_many_csr_dns",
			Template:  subjectTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"alt-backend"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"alt-backend", "localhost", "extra"},
			WantErr:   true,
		},
	}

	executeTemplateCases(t, tests)
}

func TestX509TemplateConstraintsLocalhost(t *testing.T) {
	_, localhostTpl := extractHeredocTemplates(t)

	tests := []templateTestCase{
		{
			Name:      "localhost_valid",
			Template:  localhostTpl,
			TokenSub:  "localhost",
			TokenSANs: []string{"localhost"},
			CSRCN:     "localhost",
			CSRDNS:    []string{"localhost"},
			WantErr:   false,
		},
		{
			Name:      "localhost_foreign_ott_sub",
			Template:  localhostTpl,
			TokenSub:  "alt-backend",
			TokenSANs: []string{"localhost"},
			CSRCN:     "localhost",
			CSRDNS:    []string{"localhost"},
			WantErr:   true,
		},
		{
			Name:      "localhost_foreign_ott_sans",
			Template:  localhostTpl,
			TokenSub:  "localhost",
			TokenSANs: []string{"localhost", "alt-backend"},
			CSRCN:     "localhost",
			CSRDNS:    []string{"localhost"},
			WantErr:   true,
		},
		{
			Name:      "localhost_foreign_csr_cn",
			Template:  localhostTpl,
			TokenSub:  "localhost",
			TokenSANs: []string{"localhost"},
			CSRCN:     "alt-backend",
			CSRDNS:    []string{"localhost"},
			WantErr:   true,
		},
		{
			Name:      "localhost_foreign_csr_dns",
			Template:  localhostTpl,
			TokenSub:  "localhost",
			TokenSANs: []string{"localhost"},
			CSRCN:     "localhost",
			CSRDNS:    []string{"localhost", "alt-backend"},
			WantErr:   true,
		},
		{
			Name:      "localhost_duplicate_ott_sans",
			Template:  localhostTpl,
			TokenSub:  "localhost",
			TokenSANs: []string{"localhost", "localhost"},
			CSRCN:     "localhost",
			CSRDNS:    []string{"localhost"},
			WantErr:   true,
		},
		{
			Name:      "localhost_duplicate_csr_dns",
			Template:  localhostTpl,
			TokenSub:  "localhost",
			TokenSANs: []string{"localhost"},
			CSRCN:     "localhost",
			CSRDNS:    []string{"localhost", "localhost"},
			WantErr:   true,
		},
		{
			Name:      "localhost_csr_ip_sans",
			Template:  localhostTpl,
			TokenSub:  "localhost",
			TokenSANs: []string{"localhost"},
			CSRCN:     "localhost",
			CSRDNS:    []string{"localhost"},
			CSRIPs:    []string{"127.0.0.1"},
			WantErr:   true,
		},
		{
			Name:      "localhost_csr_uri_sans",
			Template:  localhostTpl,
			TokenSub:  "localhost",
			TokenSANs: []string{"localhost"},
			CSRCN:     "localhost",
			CSRDNS:    []string{"localhost"},
			CSRURIs:   []string{"spiffe://alt/localhost"},
			WantErr:   true,
		},
		{
			Name:      "localhost_csr_email_sans",
			Template:  localhostTpl,
			TokenSub:  "localhost",
			TokenSANs: []string{"localhost"},
			CSRCN:     "localhost",
			CSRDNS:    []string{"localhost"},
			CSREmails: []string{"root@localhost"},
			WantErr:   true,
		},
	}

	executeTemplateCases(t, tests)
}

func TestBootstrapScript_UsesRestrictivePasswordPermissions(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("cannot find repo root: %v", err)
	}
	scriptPath := filepath.Join(repoRoot, "pki-agent", "scripts", "bootstrap-pki-provisioner.sh")
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("cannot read bootstrap script %s: %v", scriptPath, err)
	}
	content := string(data)
	if strings.Contains(content, "chmod 0644") {
		t.Fatal("bootstrap-pki-provisioner.sh must not use chmod 0644 for provisioner password files")
	}
	if !strings.Contains(content, "umask 0077") || !strings.Contains(content, "chmod 0600") {
		t.Fatal("bootstrap-pki-provisioner.sh must use umask 0077 and chmod 0600 for provisioner password files")
	}
}
