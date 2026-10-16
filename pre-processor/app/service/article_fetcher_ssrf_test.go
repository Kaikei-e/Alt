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
		"IPv6 loopback ::1 must be blocked":                       {"::1", true},
		"IPv6 unspecified :: must be blocked":                     {"::", true},
		"IPv6 link-local fe80:: must be blocked":                  {"fe80::1", true},
		"IPv6 ULA fc00:: must be blocked":                         {"fc00::1", true},
		"IPv6 ULA fd00:: must be blocked":                         {"fd00::1", true},
		"IPv6 ULA fdff:: must be blocked":                         {"fdff:ffff::1", true},
		"IPv6 public address must not be blocked":                 {"2001:4860:4860::8888", false}, // Google public DNS
		"IPv4-mapped loopback must be blocked":                    {"::ffff:127.0.0.1", true},
		"IPv4-mapped metadata 169.254.169.254 must be blocked":    {"::ffff:169.254.169.254", true},
		"IPv4-mapped CGNAT 100.64.0.1 must be blocked":            {"::ffff:100.64.0.1", true},
		"IPv4-mapped Alibaba metadata must be blocked":            {"::ffff:100.100.100.200", true},
		"IPv4-mapped unspecified 0.0.0.0 must be blocked":         {"::ffff:0.0.0.0", true},
		"IPv4-mapped this-network 0.1.2.3 must be blocked":        {"::ffff:0.1.2.3", true},
		"IPv4-mapped Oracle metadata 192.0.0.192 must be blocked": {"::ffff:192.0.0.192", true},
		"IPv4-mapped benchmark 198.18.0.1 must be blocked":        {"::ffff:198.18.0.1", true},
		"IPv4-mapped broadcast 255.255.255.255 must be blocked":   {"::ffff:255.255.255.255", true},
		"IPv4-mapped multicast 224.0.0.1 must be blocked":         {"::ffff:224.0.0.1", true},
		"IPv4-mapped public 8.8.8.8 must not be blocked":          {"::ffff:8.8.8.8", false},
		"IPv4 CGNAT 100.64.0.1 must be blocked":                   {"100.64.0.1", true},
		"IPv4 Alibaba metadata 100.100.100.200 must be blocked":   {"100.100.100.200", true},
		"IPv4 Oracle metadata 192.0.0.192 must be blocked":        {"192.0.0.192", true},
		"IPv4 this-network 0.1.2.3 must be blocked":               {"0.1.2.3", true},
		"IPv4 broadcast 255.255.255.255 must be blocked":          {"255.255.255.255", true},
		"IPv4-compatible ::10.0.0.1 must be blocked":              {"::10.0.0.1", true},
		"IPv6 NAT64 64:ff9b::169.254.169.254 must be blocked":     {"64:ff9b::169.254.169.254", true},
		"IPv6 6to4 2002:7f00:1:: must be blocked":                 {"2002:7f00:1::", true},
		"IPv6 site-local fec0::1 must be blocked":                 {"fec0::1", true},
		"IPv6 discard 100::1 must be blocked":                     {"100::1", true},
		"IPv6 multicast ff02::1 must be blocked":                  {"ff02::1", true},
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
		"IPv6 loopback literal is blocked":              {"https://[::1]/", true},
		"IPv6 ULA fd00 literal is blocked":              {"https://[fd00::1]/", true},
		"IPv4-mapped metadata literal is blocked":       {"https://[::ffff:169.254.169.254]/latest/meta-data/", true},
		"IPv4-mapped CGNAT literal is blocked":          {"https://[::ffff:100.64.0.1]/", true},
		"IPv4-mapped unspecified literal is blocked":    {"https://[::ffff:0.0.0.0]/", true},
		"IPv4-mapped broadcast literal is blocked":      {"https://[::ffff:255.255.255.255]/", true},
		"IPv6 public literal is allowed":                {"https://[2001:4860:4860::8888]/", false},
		"IPv4-mapped public literal 8.8.8.8 is allowed": {"https://[::ffff:8.8.8.8]/", false},
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

func TestEnvoyHTTPClient_ResolveDomain_BlocksPrivateIPs(t *testing.T) {
	c := &EnvoyHTTPClient{logger: testLoggerFetcher()}
	blockedIPs := []string{
		"127.0.0.1",
		"169.254.169.254",
		"::ffff:169.254.169.254",
		"::ffff:100.64.0.1",
		"::ffff:0.0.0.0",
		"10.0.0.1",
		"fd00::1",
	}
	for _, rawIP := range blockedIPs {
		t.Run(rawIP, func(t *testing.T) {
			withStubResolver(t, func(host string) ([]net.IP, error) {
				return []net.IP{net.ParseIP(rawIP)}, nil
			})
			_, err := c.ResolveDomain("attacker.example.com")
			if err == nil {
				t.Errorf("ResolveDomain with resolved IP %q succeeded, want blocked error", rawIP)
			}
		})
	}
}
