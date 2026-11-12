package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/labstack/echo/v4"
)

func NewTrustedProxyIPExtractor(trustedProxies []netip.Prefix) echo.IPExtractor {
	return func(req *http.Request) string {
		return ExtractClientIP(req, trustedProxies)
	}
}

func ExtractClientIP(req *http.Request, trustedProxies []netip.Prefix) string {
	if req == nil {
		return ""
	}
	rawPeer := strings.TrimSpace(req.RemoteAddr)
	host, _, err := net.SplitHostPort(rawPeer)
	if err != nil {
		host = rawPeer
	}
	peerAddr, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	peerAddr = peerAddr.Unmap()

	if !isTrustedAddr(peerAddr, trustedProxies) {
		return peerAddr.String()
	}

	xffValues := req.Header.Values("X-Forwarded-For")
	if len(xffValues) == 0 {
		return peerAddr.String()
	}

	var entries []string
	for _, raw := range xffValues {
		entries = append(entries, strings.Split(raw, ",")...)
	}

	for i := len(entries) - 1; i >= 0; i-- {
		candidate, err := netip.ParseAddr(strings.TrimSpace(entries[i]))
		if err != nil {
			return peerAddr.String()
		}
		candidate = candidate.Unmap()
		if !isTrustedAddr(candidate, trustedProxies) {
			return candidate.String()
		}
	}

	return peerAddr.String()
}

func isTrustedAddr(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
