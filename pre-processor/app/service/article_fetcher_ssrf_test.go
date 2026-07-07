package service

import (
	"net"
	"testing"
)

// TestIsPrivateIPAddress_IPv6Coverage reproduces the HIGH SSRF finding: the
// previous implementation only checked the IPv6 first two bytes against
// fe80::/16 (link-local) and fc00::/16 (half of the fc00::/7 ULA range), so
// it missed the IPv6 loopback address (::1) entirely and half of the ULA
// space (fd00::/8). Both are classic SSRF pivot targets for internal
// services listening on IPv6.
func TestIsPrivateIPAddress_IPv6Coverage(t *testing.T) {
	s := &articleFetcherService{}

	tests := map[string]struct {
		ip      string
		blocked bool
	}{
		"IPv6 loopback ::1 must be blocked":       {"::1", true},
		"IPv6 unspecified :: must be blocked":     {"::", true},
		"IPv6 link-local fe80:: must be blocked":  {"fe80::1", true},
		"IPv6 ULA fc00:: must be blocked":         {"fc00::1", true},
		"IPv6 ULA fd00:: must be blocked":         {"fd00::1", true},
		"IPv6 ULA fdff:: must be blocked":         {"fdff:ffff::1", true},
		"IPv6 public address must not be blocked": {"2001:4860:4860::8888", false}, // Google public DNS
		"IPv4-mapped loopback must be blocked":    {"::ffff:127.0.0.1", true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) returned nil", tc.ip)
			}
			got := s.isPrivateIPAddress(ip)
			if got != tc.blocked {
				t.Errorf("isPrivateIPAddress(%q) = %v, want %v", tc.ip, got, tc.blocked)
			}
		})
	}
}

// TestValidateURL_IPv6PrivateHosts exercises the same fix through the public
// ValidateURL entry point used by the fetcher's SSRF guard.
func TestValidateURL_IPv6PrivateHosts(t *testing.T) {
	tests := map[string]struct {
		url         string
		expectError bool
	}{
		"IPv6 loopback literal is blocked": {"https://[::1]/", true},
		"IPv6 ULA fd00 literal is blocked": {"https://[fd00::1]/", true},
		"IPv6 public literal is allowed":   {"https://[2001:4860:4860::8888]/", false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			svc := NewArticleFetcherService(testLoggerFetcher())
			err := svc.ValidateURL(tc.url)
			if tc.expectError && err == nil {
				t.Errorf("ValidateURL(%q) = nil error, want blocked", tc.url)
			}
			if !tc.expectError && err != nil {
				t.Errorf("ValidateURL(%q) = %v, want no error", tc.url, err)
			}
		})
	}
}
