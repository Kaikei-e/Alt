package utils

import (
	"alt/config"
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
	// Use default configuration if not provided
	cfg := &config.HTTPConfig{
		ClientTimeout:       30 * time.Second,
		DialTimeout:         10 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	}
	return SecureHTTPClientWithConfig(cfg)
}

// SecureHTTPClientWithConfig creates an HTTP client with SSRF protection using provided configuration
func SecureHTTPClientWithConfig(cfg *config.HTTPConfig) *http.Client {
	dialer := &net.Dialer{
		Timeout: cfg.DialTimeout,
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
		TLSHandshakeTimeout: cfg.TLSHandshakeTimeout,
		IdleConnTimeout:     cfg.IdleConnTimeout,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.ClientTimeout,
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
	// Allow both HTTP and HTTPS
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("only HTTP and HTTPS schemes allowed")
	}

	host := u.Hostname()
	port := u.Port()

	// Reuse target validation logic which checks private hosts and blocked ports
	if err := validateTarget(host, port); err != nil {
		return err
	}

	if u.Hostname() == "" {
		return errors.New("URL must contain a host")
	}

	return nil
}
