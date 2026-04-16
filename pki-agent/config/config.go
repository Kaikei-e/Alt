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
}

// Load parses environment variables (with _FILE support for secrets) into
// a validated Config. Returns an error if any required field is absent.
func Load() (*Config, error) {
	c := &Config{
		CAURL:           getEnv("STEP_CA_URL", "https://step-ca:9000"),
		RootFile:        getEnv("STEP_CA_ROOT_FILE", "/trust/ca-bundle.pem"),
		Provisioner:     getEnv("STEP_CA_PROVISIONER", "pki-agent"),
		PasswordFile:    getEnv("STEP_CA_PROVISIONER_PASSWORD_FILE", "/run/secrets/step_ca_root_password"),
		Subject:         getEnv("CERT_SUBJECT", ""),
		CertPath:        getEnv("CERT_PATH", "/certs/svc-cert.pem"),
		KeyPath:         getEnv("KEY_PATH", "/certs/svc-key.pem"),
		MetricsAddr:     getEnv("METRICS_ADDR", ":9510"),
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
