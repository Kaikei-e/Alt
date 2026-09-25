package security

import (
	"net"
)

// metadataIPs lists known cloud provider instance metadata IP addresses.
var metadataIPs = []string{
	"169.254.169.254", // AWS, Azure, and GCP metadata IP
	"100.100.100.200", // Alibaba Cloud metadata IP
	"192.0.0.192",     // Oracle Cloud metadata IP
}

// isMetadataIP reports whether an IP address matches a known cloud metadata IP.
func isMetadataIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	ipStr := ip.String()
	for _, m := range metadataIPs {
		if ipStr == m {
			return true
		}
	}
	return false
}

// IsPrivateIPAddress checks if an IP address is in private, loopback, or link-local ranges.
// This function is exported for use by other packages that need IP validation.
//
// The following ranges are considered private/dangerous:
//   - Loopback addresses (127.0.0.0/8, ::1)
//   - Link-local unicast (169.254.0.0/16, fe80::/10)
//   - Link-local multicast (224.0.0.0/24, ff02::/16)
//   - Private IPv4 ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
//   - Private IPv6 ranges: fc00::/7 (unique local addresses)
func IsPrivateIPAddress(ip net.IP) bool {
	if ip == nil {
		return true
	}

	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}

	// Check for IPv4 ranges (including IPv4-mapped IPv6)
	if ipv4 := ip.To4(); ipv4 != nil {
		// 0.0.0.0/8 (RFC 1122 Current network / "this host on this network")
		if ipv4[0] == 0 {
			return true
		}
		// 10.0.0.0/8 (RFC 1918)
		if ipv4[0] == 10 {
			return true
		}
		// 100.64.0.0/10 (RFC 6598 CGNAT, includes Alibaba Cloud metadata 100.100.100.200)
		if ipv4[0] == 100 && ipv4[1] >= 64 && ipv4[1] <= 127 {
			return true
		}
		// 127.0.0.0/8 (RFC 1122 Loopback)
		if ipv4[0] == 127 {
			return true
		}
		// 169.254.0.0/16 (RFC 3927 Link-local / Cloud metadata)
		if ipv4[0] == 169 && ipv4[1] == 254 {
			return true
		}
		// 172.16.0.0/12 (RFC 1918)
		if ipv4[0] == 172 && ipv4[1] >= 16 && ipv4[1] <= 31 {
			return true
		}
		// 192.0.0.0/24 (RFC 6890, includes Oracle Cloud metadata 192.0.0.192)
		if ipv4[0] == 192 && ipv4[1] == 0 && ipv4[2] == 0 {
			return true
		}
		// 192.168.0.0/16 (RFC 1918)
		if ipv4[0] == 192 && ipv4[1] == 168 {
			return true
		}
		// 198.18.0.0/15 (RFC 2544 Benchmark testing)
		if ipv4[0] == 198 && (ipv4[1] == 18 || ipv4[1] == 19) {
			return true
		}
		// 224.0.0.0/4 (RFC 1112 Multicast)
		if ipv4[0] >= 224 && ipv4[0] <= 239 {
			return true
		}
		// 240.0.0.0/4 (RFC 1112 Reserved for future use, includes broadcast 255.255.255.255)
		if ipv4[0] >= 240 {
			return true
		}
		return false
	}

	// Check for private IPv6 ranges (ip.To4() == nil)
	if ip.To16() != nil {
		// IPv4-compatible IPv6 (::/96, RFC 4291 section 2.5.5.1)
		// E.g., ::a00:1 (::10.0.0.1) or ::7f00:1 (::127.0.0.1)
		if isAllZeros(ip[0:12]) {
			return IsPrivateIPAddress(net.IPv4(ip[12], ip[13], ip[14], ip[15]))
		}
		// NAT64 well-known prefix (64:ff9b::/96, RFC 6052)
		if ip[0] == 0x00 && ip[1] == 0x64 && ip[2] == 0xff && ip[3] == 0x9b && isAllZeros(ip[4:12]) {
			return true
		}
		// 6to4 prefix (2002::/16, RFC 3056, deprecated by RFC 7526)
		if ip[0] == 0x20 && ip[1] == 0x02 {
			return true
		}
		// Unique local addresses (fc00::/7)
		if ip[0] == 0xfc || ip[0] == 0xfd {
			return true
		}
		// Site-local unicast (fec0::/10, RFC 3879, deprecated)
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0xc0 {
			return true
		}
		// Discard-only prefix (100::/64, RFC 6666)
		if ip[0] == 0x01 && ip[1] == 0x00 && isAllZeros(ip[2:8]) {
			return true
		}
	}

	return false
}

func isAllZeros(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// IsPrivateHost checks if a hostname resolves to private IP addresses.
// This function is exported for use by other packages.
//
// Returns true if:
//   - The hostname is a direct IP address in private ranges
//   - The hostname resolves to any private IP address (DNS rebinding protection)
//   - DNS resolution fails (fails closed for security)
func IsPrivateHost(hostname string) bool {
	// Try to parse as IP first
	ip := net.ParseIP(hostname)
	if ip != nil {
		return IsPrivateIPAddress(ip)
	}

	// If it's a hostname, resolve it to IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// Block on resolution failure as a security measure
		return true
	}

	// Check ALL resolved IPs (both A and AAAA records) to prevent DNS rebinding
	// If ANY resolved IP is private/dangerous, block the entire request
	for _, resolvedIP := range ips {
		if IsPrivateIPAddress(resolvedIP) {
			return true
		}
	}

	return false
}
