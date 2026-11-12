package config

import (
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strings"
)

const TrustedProxyCIDRsEnv = "TRUSTED_PROXY_CIDRS"

const TrustedProxyCIDRsNone = "none"

func ParseTrustedProxyCIDRs(raw string) ([]netip.Prefix, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("backend config: %s is required (comma-separated CIDRs or %q)", TrustedProxyCIDRsEnv, TrustedProxyCIDRsNone)
	}
	if strings.EqualFold(trimmed, TrustedProxyCIDRsNone) {
		return nil, nil
	}

	parts := strings.Split(trimmed, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			return nil, fmt.Errorf("backend config: empty CIDR entry in %s", TrustedProxyCIDRsEnv)
		}
		prefix, err := netip.ParsePrefix(token)
		if err != nil {
			return nil, fmt.Errorf("backend config: invalid CIDR %q in %s: %w", token, TrustedProxyCIDRsEnv, err)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func LoadTrustedProxyCIDRs() ([]netip.Prefix, error) {
	prefixes, err := ParseTrustedProxyCIDRs(os.Getenv(TrustedProxyCIDRsEnv))
	if err != nil {
		return nil, err
	}
	if len(prefixes) == 0 {
		slog.Warn("trusted_proxy_cidrs_disabled", "binary", "backend", "trusted_proxy_cidrs", TrustedProxyCIDRsNone)
		return nil, nil
	}
	canonical := make([]string, len(prefixes))
	for i, p := range prefixes {
		canonical[i] = p.String()
	}
	slog.Info("trusted_proxy_cidrs_loaded", "binary", "backend", "trusted_proxy_cidrs", strings.Join(canonical, ","))
	return prefixes, nil
}

func TrustedProxyCIDRsFromEnv() []netip.Prefix {
	raw := strings.TrimSpace(os.Getenv(TrustedProxyCIDRsEnv))
	if raw == "" {
		return nil
	}
	prefixes, err := ParseTrustedProxyCIDRs(raw)
	if err != nil {
		return nil
	}
	return prefixes
}

func ValidateEdgeTrust() error {
	_, err := LoadTrustedProxyCIDRs()
	return err
}
