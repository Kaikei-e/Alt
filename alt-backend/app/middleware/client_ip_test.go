package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustPrefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, raw := range cidrs {
		p, err := netip.ParsePrefix(raw)
		require.NoError(t, err)
		out = append(out, p.Masked())
	}
	return out
}

func TestExtractClientIP(t *testing.T) {
	trusted := mustPrefixes(t, "10.0.0.0/8", "172.16.0.0/12", "fd00::/8")

	t.Run("untrusted direct peer ignores X-Forwarded-For and X-Real-IP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
		req.RemoteAddr = "198.51.100.10:54321"
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		req.Header.Set("X-Real-IP", "5.6.7.8")
		require.Equal(t, "198.51.100.10", ExtractClientIP(req, trusted))
	})

	t.Run("ipv4-mapped ipv6 peer matches ipv4 trusted CIDR and unmaps XFF address", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
		req.RemoteAddr = "[::ffff:10.1.2.3]:4321"
		req.Header.Set("X-Forwarded-For", "198.51.100.99, ::ffff:203.0.113.42, ::ffff:172.16.0.4")
		require.Equal(t, "203.0.113.42", ExtractClientIP(req, trusted))
	})

	t.Run("multiple X-Forwarded-For header lines preserve wire order right-to-left", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
		req.RemoteAddr = "10.0.0.2:8080"
		req.Header.Add("X-Forwarded-For", "198.51.100.1, 203.0.113.77")
		req.Header.Add("X-Forwarded-For", "172.16.1.9")
		require.Equal(t, "203.0.113.77", ExtractClientIP(req, trusted))
	})

	t.Run("rightmost XFF entry with port is rejected without scanning spoofed left entries", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
		req.RemoteAddr = "10.0.0.2:8080"
		req.Header.Set("X-Forwarded-For", "198.51.100.1, 203.0.113.42:9000")
		require.Equal(t, "10.0.0.2", ExtractClientIP(req, trusted))
	})

	t.Run("rightmost XFF garbage entry is rejected without scanning spoofed left entries", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
		req.RemoteAddr = "10.0.0.2:8080"
		req.Header.Set("X-Forwarded-For", "198.51.100.1, not-an-ip")
		require.Equal(t, "10.0.0.2", ExtractClientIP(req, trusted))
	})

	t.Run("garbage or port-suffixed entries to the left of valid client IP are never reached", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
		req.RemoteAddr = "10.0.0.2:8080"
		req.Header.Set("X-Forwarded-For", "not-an-ip, 198.51.100.5:1234, 203.0.113.88, 172.16.0.3")
		require.Equal(t, "203.0.113.88", ExtractClientIP(req, trusted))
	})

	t.Run("all XFF hops inside trusted CIDRs falls back to peer address", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
		req.RemoteAddr = "10.0.0.2:8080"
		req.Header.Set("X-Forwarded-For", "10.1.1.1, 172.16.0.9")
		require.Equal(t, "10.0.0.2", ExtractClientIP(req, trusted))
	})

	t.Run("malformed RemoteAddr returns empty string", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
		req.RemoteAddr = "invalid-remote"
		require.Equal(t, "", ExtractClientIP(req, trusted))
	})
}
