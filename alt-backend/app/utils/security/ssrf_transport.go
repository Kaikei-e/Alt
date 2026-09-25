package security

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// CreateSecureHTTPClient creates an HTTP client with SSRF protection using connection-time validation
// This follows the Safeurl approach of validating IPs at actual connection time to prevent DNS rebinding
func (v *SSRFValidator) CreateSecureHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			return v.validateConnectionAddress(network, address)
		},
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		DisableKeepAlives:     false,
		DisableCompression:    false,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     false,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if err := v.ValidateURL(req.Context(), req.URL); err != nil {
				return fmt.Errorf("redirect blocked by SSRF policy: %w", err)
			}
			return nil
		},
	}
}

// ValidateDialIP validates the resolved connection address (host is already an
// IP, as passed to net.Dialer.Control) at dial time to defeat DNS rebinding: a
// hostname that passed pre-fetch validation can re-resolve to a private or
// cloud-metadata IP by the time the socket actually connects. Unlike
// validateConnectionAddress it does NOT enforce the 80/443 port allow-list, so
// callers that legitimately fetch from non-standard ports (RSS feeds) keep
// working; it blocks only private/dangerous and cloud-metadata IPs.
func (v *SSRFValidator) ValidateDialIP(network, address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return &ValidationError{
			Message: "invalid connection address format",
			Type:    "CONNECTION_ADDRESS_ERROR",
			Details: map[string]interface{}{"address": address, "error": err.Error()},
		}
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return &ValidationError{
			Message: "invalid IP address in connection",
			Type:    "INVALID_IP_ERROR",
			Details: map[string]interface{}{"host": host, "port": port},
		}
	}
	if IsPrivateIPAddress(ip) {
		return &ValidationError{
			Message: "connection to private/dangerous IP blocked",
			Type:    "PRIVATE_IP_BLOCKED",
			Details: map[string]interface{}{"ip": ip.String(), "host": host, "port": port},
		}
	}
	if isMetadataIP(ip) {
		return &ValidationError{
			Message: "connection to metadata endpoint IP blocked",
			Type:    "METADATA_IP_BLOCKED",
			Details: map[string]interface{}{"ip": ip.String(), "host": host, "port": port},
		}
	}
	return nil
}

// validateConnectionAddress validates IP addresses at connection time (Safeurl approach)
// This prevents DNS rebinding attacks by validating the actual IP being connected to
func (v *SSRFValidator) validateConnectionAddress(network, address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return &ValidationError{
			Message: "invalid connection address format",
			Type:    "CONNECTION_ADDRESS_ERROR",
			Details: map[string]interface{}{
				"address": address,
				"error":   err.Error(),
			},
		}
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return &ValidationError{
			Message: "invalid IP address in connection",
			Type:    "INVALID_IP_ERROR",
			Details: map[string]interface{}{
				"host": host,
				"port": port,
			},
		}
	}

	return v.validateConnectionIP(ip, host, port)
}

// validateConnectionIP performs IP-level validation at connection time
func (v *SSRFValidator) validateConnectionIP(ip net.IP, host, port string) error {
	isTestingLocalhost := v.allowTestingLocalhost &&
		(host == "127.0.0.1" || host == "::1" || strings.HasPrefix(host, "127."))

	if isTestingLocalhost {
		return nil
	}

	if IsPrivateIPAddress(ip) {
		return &ValidationError{
			Message: "connection to private/dangerous IP blocked",
			Type:    "PRIVATE_IP_BLOCKED",
			Details: map[string]interface{}{
				"ip":   ip.String(),
				"host": host,
				"port": port,
			},
		}
	}

	if isMetadataIP(ip) {
		return &ValidationError{
			Message: "connection to metadata endpoint IP blocked",
			Type:    "METADATA_IP_BLOCKED",
			Details: map[string]interface{}{
				"ip":   ip.String(),
				"host": host,
				"port": port,
			},
		}
	}

	if port != "" && !v.allowedPorts[port] {
		return &ValidationError{
			Message: fmt.Sprintf("connection to non-allowed port blocked: %s", port),
			Type:    "PORT_BLOCKED",
			Details: map[string]interface{}{
				"ip":   ip.String(),
				"host": host,
				"port": port,
			},
		}
	}

	return nil
}

// validateDNSRebinding performs DNS rebinding attack prevention
func (v *SSRFValidator) validateDNSRebinding(ctx context.Context, u *url.URL) error {
	hostname := u.Hostname()

	ips, err := v.resolveWithTimeout(ctx, hostname, 5*time.Second)
	if err != nil {
		return &ValidationError{
			Message: "DNS resolution failed",
			Type:    "DNS_RESOLUTION_ERROR",
			Details: map[string]interface{}{
				"hostname": hostname,
				"error":    err.Error(),
			},
		}
	}

	for _, ip := range ips {
		if err := v.validateResolvedIP(ip, hostname); err != nil {
			return err
		}
	}

	// Check for TOCTOU by re-resolving and comparing (only for suspicious domains)
	if isSuspiciousDomain(hostname) {
		time.Sleep(100 * time.Millisecond)
		ips2, err := v.resolveWithTimeout(ctx, hostname, 5*time.Second)
		if err == nil {
			if !v.compareIPLists(ips, ips2) {
				return &ValidationError{
					Message: "TOCTOU attack detected",
					Type:    "TOCTOU_ATTACK_BLOCKED",
					Details: map[string]interface{}{
						"hostname":    hostname,
						"initial_ips": ips,
						"second_ips":  ips2,
					},
				}
			}
		}
	}

	return nil
}

// resolveWithTimeout queries IP addresses for a hostname with an explicit timeout.
func (v *SSRFValidator) resolveWithTimeout(ctx context.Context, hostname string, timeout time.Duration) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: timeout,
			}
			return d.DialContext(ctx, network, address)
		},
	}

	addrs, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, err
	}

	ips := make([]net.IP, len(addrs))
	for i, addr := range addrs {
		ips[i] = addr.IP
	}

	return ips, nil
}

// validateResolvedIP ensures the resolved IP address is not private or dangerous.
func (v *SSRFValidator) validateResolvedIP(ip net.IP, hostname string) error {
	isTestingLocalhost := v.allowTestingLocalhost &&
		(hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127."))

	if !isTestingLocalhost && IsPrivateIPAddress(ip) {
		return &ValidationError{
			Message: "DNS rebinding attack detected",
			Type:    "DNS_REBINDING_BLOCKED",
			Details: map[string]interface{}{
				"hostname":    hostname,
				"resolved_ip": ip.String(),
			},
		}
	}

	return nil
}

// compareIPLists determines whether two IP address sets are equal.
func (v *SSRFValidator) compareIPLists(ips1, ips2 []net.IP) bool {
	if len(ips1) != len(ips2) {
		return false
	}

	for i, ip1 := range ips1 {
		if !ip1.Equal(ips2[i]) {
			return false
		}
	}

	return true
}
