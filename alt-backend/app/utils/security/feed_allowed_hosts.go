package security

import (
	"os"
	"strings"
)

const feedAllowedHostsEnv = "FEED_ALLOWED_HOSTS"

// IsFeedHostAllowed reports whether the hostname is explicitly allowed via FEED_ALLOWED_HOSTS.
func IsFeedHostAllowed(hostname string) bool {
	normalizedHost := normalizeAllowedHost(hostname)
	if normalizedHost == "" {
		return false
	}

	for _, allowed := range strings.Split(os.Getenv(feedAllowedHostsEnv), ",") {
		if normalizeAllowedHost(allowed) == normalizedHost {
			return true
		}
	}

	return false
}

func normalizeAllowedHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
