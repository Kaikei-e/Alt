// Package config parses the agent's env / secret-file inputs into a typed
// struct. Mirrors auth-hub/config/config.go's _FILE suffix pattern so the
// same Docker secret idiom works.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// step-ca endpoint and trust.
	CAURL    string
	RootFile string

	// Provisioner for minting OTTs.
	Provisioner  string
	PasswordFile string

	// Cert identity.
	Subject string
	SANs    []string

	// Output paths on the shared volume.
	CertPath string
	KeyPath  string
	OwnerUID int
	OwnerGID int

	// Rotation policy.
	RenewAtFraction float64
	TickInterval    time.Duration

	// Observability.
	MetricsAddr string

	// Optional TLS reverse proxy mode. Set PROXY_LISTEN to enable.
	ProxyListen       string
	ProxyUpstream     string
	ProxyVerifyClient bool
	ProxyAllowedPeers []string
	ProxyCAPath       string
	// ProxyResponseHeaderTimeout bounds how long the proxy waits for the
	// upstream's response headers. Raise it for upstreams that run LLM
	// inference before answering; keep it tight everywhere else.
	ProxyResponseHeaderTimeout time.Duration
}

// ProvisionerName is the Wave 4 JWK name for a CERT_SUBJECT. Distinct from
// the shared "pki-agent" provisioner (F-001 blast radius).
func ProvisionerName(subject string) string {
	return "pki-agent-" + subject
}

// ProvisionerPasswordFile is the in-container secret path for a subject's
// JWK password. It must never be the shared step_ca_root_password file.
func ProvisionerPasswordFile(subject string) string {
	return "/run/secrets/pki-agent-" + subject + "-jwk"
}

// Load parses environment variables (with _FILE support for secrets) into
// a validated Config. Returns an error if any required field is absent.
func Load() (*Config, error) {
	subject := getEnv("CERT_SUBJECT", "")
	c := &Config{
		CAURL:        getEnv("STEP_CA_URL", "https://step-ca:9000"),
		RootFile:     getEnv("STEP_CA_ROOT_FILE", "/trust/ca-bundle.pem"),
		Subject:      subject,
		Provisioner:  getEnv("STEP_CA_PROVISIONER", ProvisionerName(subject)),
		PasswordFile: getEnv("STEP_CA_PROVISIONER_PASSWORD_FILE", ProvisionerPasswordFile(subject)),
		CertPath:     getEnv("CERT_PATH", "/certs/svc-cert.pem"),
		KeyPath:      getEnv("KEY_PATH", "/certs/svc-key.pem"),
		MetricsAddr:  getEnv("METRICS_ADDR", ":9510"),
	}
	if s := getEnv("CERT_SANS", ""); s != "" {
		for _, part := range strings.Split(s, ",") {
			if p := strings.TrimSpace(part); p != "" {
				c.SANs = append(c.SANs, p)
			}
		}
	}

	uid, err := strconv.Atoi(getEnv("CERT_OWNER_UID", "0"))
	if err != nil {
		return nil, fmt.Errorf("CERT_OWNER_UID: %w", err)
	}
	c.OwnerUID = uid
	gid, err := strconv.Atoi(getEnv("CERT_OWNER_GID", strconv.Itoa(uid)))
	if err != nil {
		return nil, fmt.Errorf("CERT_OWNER_GID: %w", err)
	}
	c.OwnerGID = gid

	frac, err := strconv.ParseFloat(getEnv("RENEW_AT_FRACTION", "0.66"), 64)
	if err != nil {
		return nil, fmt.Errorf("RENEW_AT_FRACTION: %w", err)
	}
	if frac <= 0 || frac >= 1 {
		return nil, fmt.Errorf("RENEW_AT_FRACTION must be in (0,1), got %v", frac)
	}
	c.RenewAtFraction = frac

	tick, err := time.ParseDuration(getEnv("TICK_INTERVAL", "5m"))
	if err != nil {
		return nil, fmt.Errorf("TICK_INTERVAL: %w", err)
	}
	c.TickInterval = tick

	c.ProxyListen = getEnv("PROXY_LISTEN", "")
	c.ProxyUpstream = getEnv("PROXY_UPSTREAM", "")
	c.ProxyCAPath = getEnv("PROXY_CA_FILE", c.RootFile)
	c.ProxyVerifyClient = strings.EqualFold(getEnv("PROXY_VERIFY_CLIENT", "off"), "on")
	respHeaderTimeout, err := time.ParseDuration(getEnv("PROXY_RESPONSE_HEADER_TIMEOUT", "15s"))
	if err != nil {
		return nil, fmt.Errorf("PROXY_RESPONSE_HEADER_TIMEOUT: %w", err)
	}
	c.ProxyResponseHeaderTimeout = respHeaderTimeout
	if peers := getEnv("PROXY_ALLOWED_PEERS", ""); peers != "" {
		for _, p := range strings.Split(peers, ",") {
			if s := strings.TrimSpace(p); s != "" {
				c.ProxyAllowedPeers = append(c.ProxyAllowedPeers, s)
			}
		}
	}

	return c, c.validate()
}

func (c *Config) validate() error {
	if c.Subject == "" {
		return errors.New("CERT_SUBJECT is required")
	}
	if len(c.SANs) == 0 {
		c.SANs = []string{c.Subject}
	}
	if len(c.ProxyAllowedPeers) > 0 && !c.ProxyVerifyClient {
		return errors.New("PROXY_ALLOWED_PEERS requires PROXY_VERIFY_CLIENT=on: an allowlist without mTLS client verification cannot be enforced")
	}
	return nil
}

// getEnv returns the value of key, with _FILE suffix support: if key_FILE
// is set and points to a readable file, the file contents (trimmed) are
// used. Otherwise the env value or fallback.
func getEnv(key, fallback string) string {
	if fileRef := os.Getenv(key + "_FILE"); fileRef != "" {
		if b, err := os.ReadFile(fileRef); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
