package config

import (
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strings"
)

// MaxRequestBodyBytes bounds REST request bodies and Connect messages (1 MB OPML import plus multipart overhead; 1 MiB HTML-extracted article cap).
const MaxRequestBodyBytes = 2 << 20

const (
	trustedProxyCIDRsEnv  = "TRUSTED_PROXY_CIDRS"
	trustedProxyCIDRsNone = "none"
)

func ParseTrustedProxyCIDRs(raw string) ([]netip.Prefix, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("backend config: %s is required (comma-separated CIDRs or %q)", trustedProxyCIDRsEnv, trustedProxyCIDRsNone)
	}
	if strings.EqualFold(trimmed, trustedProxyCIDRsNone) {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			return nil, fmt.Errorf("backend config: empty CIDR entry in %s", trustedProxyCIDRsEnv)
		}
		prefix, err := netip.ParsePrefix(token)
		if err != nil {
			return nil, fmt.Errorf("backend config: invalid CIDR %q in %s: %w", token, trustedProxyCIDRsEnv, err)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func LoadTrustedProxyCIDRs() ([]netip.Prefix, error) {
	raw, ok := os.LookupEnv(trustedProxyCIDRsEnv)
	if !ok {
		return nil, fmt.Errorf("backend config: %s is not set (set comma-separated CIDRs or %q)", trustedProxyCIDRsEnv, trustedProxyCIDRsNone)
	}
	return ParseTrustedProxyCIDRs(raw)
}

func ValidateEdgeTrust() error {
	prefixes, err := LoadTrustedProxyCIDRs()
	if err != nil {
		return err
	}
	if len(prefixes) == 0 {
		slog.Warn("trusted_proxy_cidrs_disabled", "binary", "backend", "trusted_proxy_cidrs", trustedProxyCIDRsNone)
		return nil
	}
	canonical := make([]string, len(prefixes))
	for i, p := range prefixes {
		canonical[i] = p.String()
	}
	slog.Info("trusted_proxy_cidrs_loaded", "binary", "backend", "trusted_proxy_cidrs", strings.Join(canonical, ","))
	return nil
}
