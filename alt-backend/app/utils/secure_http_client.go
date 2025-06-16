package utils

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SecureHTTPClient creates an HTTP client with SSRF protection
func SecureHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// Validate the target before making the connection
			if err := validateTarget(host, port); err != nil {
				return nil, err
			}

			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

// validateTarget validates the target host and port for SSRF protection
func validateTarget(host, port string) error {
	// Block common internal ports
	blockedPorts := map[string]bool{
		"22":    true, // SSH
		"23":    true, // Telnet
		"25":    true, // SMTP
		"53":    true, // DNS
		"80":    true, // HTTP (if we want to force HTTPS only)
		"110":   true, // POP3
		"143":   true, // IMAP
		"993":   true, // IMAPS
		"995":   true, // POP3S
		"1433":  true, // MSSQL
		"3306":  true, // MySQL
		"5432":  true, // PostgreSQL
		"6379":  true, // Redis
		"11211": true, // Memcached
	}

	if blockedPorts[port] {
		return errors.New("access to this port is not allowed")
	}

	// Check if the host resolves to a private IP
	if isPrivateHost(host) {
		return errors.New("access to private networks not allowed")
	}

	return nil
}

// isPrivateHost checks if a hostname resolves to private IP addresses
func isPrivateHost(hostname string) bool {
	// Try to parse as IP first
	ip := net.ParseIP(hostname)
	if ip != nil {
		return isPrivateIPAddress(ip)
	}

	// Block localhost variations
	hostname = strings.ToLower(hostname)
	if hostname == "localhost" || strings.HasPrefix(hostname, "127.") {
		return true
	}

	// Block metadata endpoints
	if hostname == "169.254.169.254" || hostname == "metadata.google.internal" {
		return true
	}

	// Block common internal domains
	internalDomains := []string{".local", ".internal", ".corp", ".lan"}
	for _, domain := range internalDomains {
		if strings.HasSuffix(hostname, domain) {
			return true
		}
	}

	// If it's a hostname, resolve it to IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// Block on resolution failure as a security measure
		return true
	}

	// Check if any resolved IP is private
	for _, ip := range ips {
		if isPrivateIPAddress(ip) {
			return true
		}
	}

	return false
}

// isPrivateIPAddress checks if an IP address is in private ranges
func isPrivateIPAddress(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private IPv4 ranges
	if ip.To4() != nil {
		// 10.0.0.0/8
		if ip[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip[0] == 192 && ip[1] == 168 {
			return true
		}
	}

	// Check for private IPv6 ranges
	if ip.To16() != nil && ip.To4() == nil {
		// Check for unique local addresses (fc00::/7)
		if ip[0] == 0xfc || ip[0] == 0xfd {
			return true
		}
	}

	return false
}

// ValidateURL validates a URL for SSRF protection
func ValidateURL(u *url.URL) error {
	// Only allow HTTPS
	if u.Scheme != "https" {
		return errors.New("only HTTPS schemes allowed")
	}

	// Block private networks
	if isPrivateHost(u.Hostname()) {
		return errors.New("access to private networks not allowed")
	}

	return nil
}
