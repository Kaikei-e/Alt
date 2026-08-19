// Package pki is the in-process certificate lifecycle for pre-processor.
//
// It is intentionally independent of the pki-agent module and of alt-backend's
// copy: no replace directive, no shared Go import. The state machine, atomic
// writes and subject-scoped config live here and are tested with a fake
// issuer, fake clock and a temp filesystem. Minting against step-ca uses
// NativeStepCAIssuer (official jose + provision token APIs, no step CLI)
// when PKI_ENROLLMENT=enabled.
package pki

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// ModeEnabled runs initial enroll + the renewal loop.
	ModeEnabled = "enabled"
	// ModeDisabled leaves cert files to the pki-agent sidecar.
	ModeDisabled = "disabled"

	defaultRenewAt = 0.66
	defaultTick    = 5 * time.Minute
	defaultBackoff = time.Second
	defaultRetries = 5

	maxResponseBytes     = 2 << 20
	maxProvisionerPages  = 20
	defaultOpsListenAddr = "127.0.0.1:9110"
	opsListenEnv         = "OPS_LISTEN"
)

var (
	// ErrSharedProvisioner is F-001: a workload must not mint with the shared JWK.
	ErrSharedProvisioner = errors.New("pki: shared provisioner pki-agent is forbidden for in-process enrollment")
	// ErrSharedRootSecret is F-001: a workload must not read the CA root password.
	ErrSharedRootSecret = errors.New("pki: provisioner password file must not be the shared step_ca_root_password secret")
	// ErrInsecureCAURL is F-001/F-002: STEP_CA_URL must be https with a host.
	ErrInsecureCAURL = errors.New("pki: STEP_CA_URL must use https")
	// ErrRedirect is F-002: the CA HTTP client never follows redirects.
	ErrRedirect = errors.New("pki: refusing HTTP redirect to step-ca")
	// ErrResponseTooLarge is a CA body exceeded maxResponseBytes.
	ErrResponseTooLarge = errors.New("pki: CA response exceeded size cap")
	// ErrProvisionerPageLimit is /provisioners pagination exceeded the page cap.
	ErrProvisionerPageLimit = errors.New("pki: provisioner listing exceeded page cap")
	// ErrPasswordTooLarge is the provisioner password file exceeded 4KiB.
	ErrPasswordTooLarge = errors.New("pki: provisioner password file exceeded size cap")
	// ErrPasswordEmpty is the provisioner password file was empty after trim.
	ErrPasswordEmpty = errors.New("pki: provisioner password file is empty")
	// ErrPasswordUnreadable is the provisioner password file could not be read as a regular file.
	ErrPasswordUnreadable = errors.New("pki: provisioner password file is unreadable")
	// ErrPasswordFileName is the provisioner password file is not the subject-scoped JWK secret.
	ErrPasswordFileName = errors.New("pki: provisioner password file is not the subject-scoped JWK secret")
	// ErrCARejected is step-ca returned 4xx (status only; no body).
	ErrCARejected = errors.New("pki: CA rejected the request")
	// ErrCAUnavailable is step-ca returned 5xx (status only; no body).
	ErrCAUnavailable = errors.New("pki: CA unavailable")
)

// Config is the typed enrollment input for one binary/subject.
type Config struct {
	Mode            string
	Subject         string
	SANs            []string
	CertPath        string
	KeyPath         string
	CAURL           string
	RootFile        string
	Provisioner     string
	PasswordFile    string
	RenewAtFraction float64
	TickInterval    time.Duration
	RetryBackoff    time.Duration
	RetryAttempts   int
}

// ProvisionerName is the Wave 4 JWK name for a CERT_SUBJECT.
func ProvisionerName(subject string) string {
	return "pki-agent-" + subject
}

// ProvisionerPasswordFile is the in-container secret path for a subject.
func ProvisionerPasswordFile(subject string) string {
	return "/run/secrets/pki-agent-" + subject + "-jwk"
}

func provisionerPasswordBasename(subject string) string {
	return "pki-agent-" + subject + "-jwk"
}

// LoadConfig reads enrollment env vars.
//
// Unset PKI_ENROLLMENT (and unset PKI_ENROLLMENT_FILE) defaults to disabled.
// That is temporary migration compatibility for image-first mixed-mode rollout
// (sidecar still owns cert files). Final compose cutover MUST set
// PKI_ENROLLMENT=enabled explicitly. An empty PKI_ENROLLMENT="" is garbage and
// fails. If any KEY_FILE is set, a missing/unreadable/empty file is an error
// (no silent fallback to env or defaults).
func LoadConfig(serviceName string) (*Config, error) {
	mode, err := loadEnrollmentMode()
	if err != nil {
		return nil, err
	}

	subject, err := getEnv("CERT_SUBJECT", serviceName)
	if err != nil {
		return nil, err
	}
	certPath, err := getEnv("CERT_PATH", "/certs/svc-cert.pem")
	if err != nil {
		return nil, err
	}
	keyPath, err := getEnv("KEY_PATH", "/certs/svc-key.pem")
	if err != nil {
		return nil, err
	}
	caURL, err := getEnv("STEP_CA_URL", "https://step-ca:9000")
	if err != nil {
		return nil, err
	}
	rootFile, err := getEnv("STEP_CA_ROOT_FILE", "/trust/ca-bundle.pem")
	if err != nil {
		return nil, err
	}
	provisioner, err := getEnv("STEP_CA_PROVISIONER", ProvisionerName(subject))
	if err != nil {
		return nil, err
	}
	passwordFile, err := getEnv("STEP_CA_PROVISIONER_PASSWORD_FILE", ProvisionerPasswordFile(subject))
	if err != nil {
		return nil, err
	}
	c := &Config{
		Mode:            mode,
		Subject:         subject,
		CertPath:        certPath,
		KeyPath:         keyPath,
		CAURL:           caURL,
		RootFile:        rootFile,
		Provisioner:     provisioner,
		PasswordFile:    passwordFile,
		RenewAtFraction: defaultRenewAt,
		TickInterval:    defaultTick,
		RetryBackoff:    defaultBackoff,
		RetryAttempts:   defaultRetries,
	}
	sansCSV, err := getEnv("CERT_SANS", "")
	if err != nil {
		return nil, err
	}
	if sansCSV != "" {
		for _, part := range strings.Split(sansCSV, ",") {
			if p := strings.TrimSpace(part); p != "" {
				c.SANs = append(c.SANs, p)
			}
		}
	}
	if len(c.SANs) == 0 && c.Subject != "" {
		c.SANs = []string{c.Subject}
	}
	if v, err := getEnv("RENEW_AT_FRACTION", ""); err != nil {
		return nil, err
	} else if v != "" {
		frac, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("pki: RENEW_AT_FRACTION: %w", err)
		}
		c.RenewAtFraction = frac
	}
	if v, err := getEnv("PKI_ENROLLMENT_TICK_INTERVAL", ""); err != nil {
		return nil, err
	} else if v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("pki: PKI_ENROLLMENT_TICK_INTERVAL: %w", err)
		}
		c.TickInterval = d
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func loadEnrollmentMode() (string, error) {
	if _, ok := os.LookupEnv("PKI_ENROLLMENT_FILE"); ok {
		raw, err := getEnv("PKI_ENROLLMENT", "")
		if err != nil {
			return "", err
		}
		mode := strings.ToLower(strings.TrimSpace(raw))
		if mode != ModeEnabled && mode != ModeDisabled {
			return "", fmt.Errorf("pki: PKI_ENROLLMENT=%q must be %q or %q", mode, ModeEnabled, ModeDisabled)
		}
		return mode, nil
	}
	v, ok := os.LookupEnv("PKI_ENROLLMENT")
	if !ok {
		// Temporary image-first compatibility: sidecar still owns cert files
		// until compose cutover sets PKI_ENROLLMENT=enabled.
		return ModeDisabled, nil
	}
	mode := strings.ToLower(strings.TrimSpace(v))
	if mode != ModeEnabled && mode != ModeDisabled {
		return "", fmt.Errorf("pki: PKI_ENROLLMENT=%q must be %q or %q", v, ModeEnabled, ModeDisabled)
	}
	return mode, nil
}

func (c *Config) validate() error {
	if c.Subject == "" {
		return errors.New("pki: CERT_SUBJECT is required")
	}
	if c.RenewAtFraction <= 0 || c.RenewAtFraction >= 1 {
		return fmt.Errorf("pki: RENEW_AT_FRACTION must be in (0,1), got %v", c.RenewAtFraction)
	}
	if c.Mode != ModeEnabled {
		return nil
	}
	if c.Provisioner == "pki-agent" || c.Provisioner == "" {
		return fmt.Errorf("%w (got %q)", ErrSharedProvisioner, c.Provisioner)
	}
	wantProv := ProvisionerName(c.Subject)
	if c.Provisioner != wantProv {
		return fmt.Errorf("pki: provisioner %q must be exactly %q", c.Provisioner, wantProv)
	}
	if strings.Contains(c.PasswordFile, "step_ca_root_password") {
		return ErrSharedRootSecret
	}
	wantBase := provisionerPasswordBasename(c.Subject)
	if filepath.Base(c.PasswordFile) != wantBase {
		return ErrPasswordFileName
	}
	cleaned := filepath.Clean(c.PasswordFile)
	if filepath.Dir(cleaned) == "/run/secrets" && cleaned != ProvisionerPasswordFile(c.Subject) {
		return ErrPasswordFileName
	}
	for _, pair := range []struct{ name, val string }{
		{"CERT_PATH", c.CertPath},
		{"KEY_PATH", c.KeyPath},
		{"STEP_CA_URL", c.CAURL},
		{"STEP_CA_ROOT_FILE", c.RootFile},
		{"STEP_CA_PROVISIONER_PASSWORD_FILE", c.PasswordFile},
	} {
		if strings.TrimSpace(pair.val) == "" {
			return fmt.Errorf("pki: %s is required when PKI_ENROLLMENT=enabled", pair.name)
		}
	}
	if err := requireHTTPS(c.CAURL); err != nil {
		return err
	}
	return nil
}

func requireHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%w (got %q)", ErrInsecureCAURL, raw)
	}
	return nil
}

// getEnv reads KEY, or KEY_FILE when that is explicitly set.
// A set KEY_FILE that is missing, unreadable, or empty is an error.
func getEnv(key, fallback string) (string, error) {
	if fileRef, ok := os.LookupEnv(key + "_FILE"); ok {
		if strings.TrimSpace(fileRef) == "" {
			return "", fmt.Errorf("pki: %s_FILE is empty", key)
		}
		b, err := readRegularNoFollow(fileRef, maxEnvFileBytes)
		if err != nil {
			return "", fmt.Errorf("pki: read %s_FILE: %w", key, err)
		}
		s := strings.TrimSpace(string(b))
		if s == "" {
			return "", fmt.Errorf("pki: %s_FILE is empty", key)
		}
		return s, nil
	}
	if v := os.Getenv(key); v != "" {
		return v, nil
	}
	return fallback, nil
}

// Issuer mints a freshly-signed leaf. Implementations MUST generate a new
// private key on every call.
type Issuer interface {
	Issue(ctx context.Context, subject string, sans []string) (certPEM, keyPEM []byte, err error)
}

// RekeyIssuer rotates a still-valid leaf via the CA rekey endpoint
// (client-certificate auth + a new key). Expired certs must use Issue.
type RekeyIssuer interface {
	Rekey(ctx context.Context, certPEM, keyPEM []byte, subject string, sans []string) (newCertPEM, newKeyPEM []byte, err error)
}
