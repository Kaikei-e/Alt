package security

import (
	"context"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSRFValidator_PrivateIPRanges(t *testing.T) {
	validator := NewSSRFValidator()

	tests := []struct {
		name      string
		url       string
		wantErr   bool
		errorType string
	}{
		// Private IPv4 ranges - 10.0.0.0/8
		{
			name:      "private IP 10.0.0.1",
			url:       "http://10.0.0.1/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		{
			name:      "private IP 10.255.255.255",
			url:       "http://10.255.255.255/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		// Private IPv4 ranges - 172.16.0.0/12
		{
			name:      "private IP 172.16.0.1",
			url:       "http://172.16.0.1/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		{
			name:      "private IP 172.31.255.255",
			url:       "http://172.31.255.255/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		// 172.15.x.x and 172.32.x.x should be allowed (not in private range)
		{
			name:    "non-private IP 172.15.0.1",
			url:     "http://172.15.0.1/api",
			wantErr: false,
		},
		{
			name:    "non-private IP 172.32.0.1",
			url:     "http://172.32.0.1/api",
			wantErr: false,
		},
		// Private IPv4 ranges - 192.168.0.0/16
		{
			name:      "private IP 192.168.0.1",
			url:       "http://192.168.0.1/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		{
			name:      "private IP 192.168.255.255",
			url:       "http://192.168.255.255/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		// Loopback addresses
		{
			name:      "loopback 127.0.0.1",
			url:       "http://127.0.0.1/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		{
			name:      "loopback 127.0.0.2",
			url:       "http://127.0.0.2/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		{
			name:      "localhost hostname",
			url:       "http://localhost/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		// Link-local addresses
		{
			name:      "link-local 169.254.0.1",
			url:       "http://169.254.0.1/api",
			wantErr:   true,
			errorType: "DNS_REBINDING_BLOCKED",
		},
		// Public IP should pass
		{
			name:    "public IP 8.8.8.8",
			url:     "http://8.8.8.8/api",
			wantErr: false,
		},
		{
			name:    "public IP 1.1.1.1",
			url:     "http://1.1.1.1/api",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = validator.ValidateURL(context.Background(), u)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorType != "" {
					assert.Contains(t, err.Error(), tt.errorType)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSSRFValidator_IsPrivateOrDangerous(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Private IPv4 ranges
		{"10.0.0.1", "10.0.0.1", true},
		{"172.16.0.1", "172.16.0.1", true},
		{"172.31.255.255", "172.31.255.255", true},
		{"192.168.1.1", "192.168.1.1", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"169.254.169.254", "169.254.169.254", true},
		// Public IPs
		{"8.8.8.8", "8.8.8.8", false},
		{"1.1.1.1", "1.1.1.1", false},
		{"172.15.0.1", "172.15.0.1", false},
		{"172.32.0.1", "172.32.0.1", false},
		// IPv6 private ranges
		{"fc00::1", "fc00::1", true},
		{"fd00::1", "fd00::1", true},
		{"::1", "::1", true},         // IPv6 loopback
		{"fe80::1", "fe80::1", true}, // Link-local
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "failed to parse IP: %s", tt.ip)

			result := IsPrivateIPAddress(ip)
			assert.Equal(t, tt.expected, result, "IP: %s", tt.ip)
		})
	}
}

func TestIsPrivateIPAddress(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Private IPv4 ranges - 10.0.0.0/8
		{"10.0.0.1 is private", "10.0.0.1", true},
		{"10.255.255.255 is private", "10.255.255.255", true},

		// Private IPv4 ranges - 172.16.0.0/12
		{"172.16.0.1 is private", "172.16.0.1", true},
		{"172.31.255.255 is private", "172.31.255.255", true},
		{"172.15.0.1 is not private", "172.15.0.1", false},
		{"172.32.0.1 is not private", "172.32.0.1", false},

		// Private IPv4 ranges - 192.168.0.0/16
		{"192.168.0.1 is private", "192.168.0.1", true},
		{"192.168.255.255 is private", "192.168.255.255", true},

		// Loopback addresses
		{"127.0.0.1 is loopback", "127.0.0.1", true},
		{"127.0.0.2 is loopback", "127.0.0.2", true},
		{"127.255.255.255 is loopback", "127.255.255.255", true},

		// Link-local addresses
		{"169.254.0.1 is link-local", "169.254.0.1", true},
		{"169.254.169.254 is link-local (AWS metadata)", "169.254.169.254", true},

		// Public IPs
		{"8.8.8.8 is public", "8.8.8.8", false},
		{"1.1.1.1 is public", "1.1.1.1", false},
		{"93.184.216.34 is public", "93.184.216.34", false},

		// IPv6 loopback
		{"::1 is loopback", "::1", true},

		// IPv6 unique local (fc00::/7)
		{"fc00::1 is unique local", "fc00::1", true},
		{"fd00::1 is unique local", "fd00::1", true},

		// IPv6 link-local
		{"fe80::1 is link-local", "fe80::1", true},

		// IPv6 public
		{"2001:4860:4860::8888 is public", "2001:4860:4860::8888", false},

		// Unspecified addresses
		{"0.0.0.0 is unspecified/current network", "0.0.0.0", true},
		{"0.0.0.1 is in 0.0.0.0/8 range", "0.0.0.1", true},
		{":: is IPv6 unspecified", "::", true},

		// CGNAT (100.64.0.0/10)
		{"100.64.0.1 is CGNAT", "100.64.0.1", true},
		{"100.100.100.200 is CGNAT (Alibaba metadata)", "100.100.100.200", true},
		{"100.127.255.255 is CGNAT", "100.127.255.255", true},
		{"100.63.255.255 is not CGNAT", "100.63.255.255", false},
		{"100.128.0.1 is not CGNAT", "100.128.0.1", false},

		// Multicast
		{"224.0.0.1 is IPv4 multicast", "224.0.0.1", true},
		{"239.255.255.250 is IPv4 multicast", "239.255.255.250", true},
		{"ff02::1 is IPv6 multicast", "ff02::1", true},
		{"ff05::2 is IPv6 multicast", "ff05::2", true},

		// Broadcast and Reserved (240.0.0.0/4)
		{"255.255.255.255 is broadcast", "255.255.255.255", true},
		{"240.0.0.1 is reserved", "240.0.0.1", true},

		// IPv4-mapped IPv6 forms
		{"::ffff:127.0.0.1 is IPv4-mapped loopback", "::ffff:127.0.0.1", true},
		{"::ffff:10.0.0.1 is IPv4-mapped private", "::ffff:10.0.0.1", true},
		{"::ffff:172.16.0.1 is IPv4-mapped private", "::ffff:172.16.0.1", true},
		{"::ffff:192.168.1.1 is IPv4-mapped private", "::ffff:192.168.1.1", true},
		{"::ffff:169.254.169.254 is IPv4-mapped link-local", "::ffff:169.254.169.254", true},
		{"::ffff:0.0.0.0 is IPv4-mapped unspecified", "::ffff:0.0.0.0", true},
		{"::ffff:100.64.0.1 is IPv4-mapped CGNAT", "::ffff:100.64.0.1", true},
		{"::ffff:224.0.0.1 is IPv4-mapped multicast", "::ffff:224.0.0.1", true},
		{"::ffff:8.8.8.8 is IPv4-mapped public", "::ffff:8.8.8.8", false},

		// IPv4-compatible IPv6 (RFC 4291)
		{"::a00:1 is IPv4-compatible 10.0.0.1", "::a00:1", true},
		{"::7f00:1 is IPv4-compatible 127.0.0.1", "::7f00:1", true},

		// NAT64 well-known prefix (64:ff9b::/96)
		{"64:ff9b::1 is NAT64", "64:ff9b::1", true},
		{"64:ff9b::a00:1 is NAT64", "64:ff9b::a00:1", true},

		// 6to4 prefix (2002::/16)
		{"2002::1 is 6to4", "2002::1", true},
		{"2002:a00:1:: is 6to4", "2002:a00:1::", true},

		// Site-local IPv6 (fec0::/10)
		{"fec0::1 is site-local", "fec0::1", true},
		{"feff::1 is site-local", "feff::1", true},

		// Discard prefix (100::/64, RFC 6666)
		{"0100::1 is discard-only prefix", "0100::1", true},
		{"0100:1::1 is not in discard-only 100::/64", "0100:1::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "failed to parse IP: %s", tt.ip)

			result := IsPrivateIPAddress(ip)
			assert.Equal(t, tt.expected, result, "IP: %s", tt.ip)
		})
	}
}

func TestIsPrivateHost(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		expected bool
	}{
		// Direct IP addresses
		{"10.0.0.1 IP is private", "10.0.0.1", true},
		{"192.168.1.1 IP is private", "192.168.1.1", true},
		{"127.0.0.1 IP is loopback", "127.0.0.1", true},
		{"8.8.8.8 IP is public", "8.8.8.8", false},
		{"1.1.1.1 IP is public", "1.1.1.1", false},

		// IPv6 addresses
		{"::1 is IPv6 loopback", "::1", true},
		{"fc00::1 is IPv6 unique local", "fc00::1", true},

		// Hostnames that fail DNS resolution (blocked by default)
		{"invalid.nonexistent.domain blocks on DNS failure", "invalid.nonexistent.domain.invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPrivateHost(tt.hostname)
			assert.Equal(t, tt.expected, result, "hostname: %s", tt.hostname)
		})
	}
}

func TestIsMetadataIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"nil ip", "", false},
		{"aws metadata", "169.254.169.254", true},
		{"alibaba metadata", "100.100.100.200", true},
		{"oracle metadata", "192.0.0.192", true},
		{"normal private ip", "192.168.1.1", false},
		{"google dns", "8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ip net.IP
			if tt.ip != "" {
				ip = net.ParseIP(tt.ip)
				require.NotNil(t, ip)
			}
			assert.Equal(t, tt.expected, isMetadataIP(ip))
		})
	}
}

func BenchmarkIsPrivateIPAddress(b *testing.B) {
	ip := net.ParseIP("192.168.1.1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsPrivateIPAddress(ip)
	}
}

func BenchmarkIsPrivateHost(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsPrivateHost("192.168.1.1")
	}
}
