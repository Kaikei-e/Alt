package url_validator

import (
	"fmt"
	"net"
	"net/url"
)

// IsAllowedURL checks if the URL is allowed (not private IP, valid scheme).
func IsAllowedURL(u *url.URL) error {
	// Allow http and https
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme not allowed: %s", u.Scheme)
	}

	// Resolve IP
	ips, err := net.LookupIP(u.Hostname())
	if err != nil {
		return fmt.Errorf("could not resolve hostname: %w", err)
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() {
			return fmt.Errorf("private IP not allowed: %s", ip.String())
		}
	}

	return nil
}
