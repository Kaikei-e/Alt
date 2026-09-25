package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractClientIP_UntrustedHeadersIgnored(t *testing.T) {
	header := http.Header{}
	header.Set("X-Forwarded-For", "1.2.3.4")
	header.Set("X-Real-IP", "5.6.7.8")

	ip := extractClientIP(header, "10.0.0.1:12345", false)
	assert.Equal(t, "10.0.0.1", ip)
}

func TestExtractClientIP_TrustedXRealIP(t *testing.T) {
	header := http.Header{}
	header.Set("X-Real-IP", "5.6.7.8")

	ip := extractClientIP(header, "10.0.0.1:12345", true)
	assert.Equal(t, "5.6.7.8", ip)
}

func TestExtractClientIP_TrustedXForwardedForLeftmost(t *testing.T) {
	header := http.Header{}
	header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")

	ip := extractClientIP(header, "10.0.0.1:12345", true)
	assert.Equal(t, "1.2.3.4", ip)
}

func TestExtractClientIP_TrustedInvalidHeaderFallsBackToRemoteAddr(t *testing.T) {
	header := http.Header{}
	header.Set("X-Real-IP", "not-an-ip")
	header.Set("X-Forwarded-For", "invalid, also-invalid")

	ip := extractClientIP(header, "10.0.0.1:12345", true)
	assert.Equal(t, "10.0.0.1", ip)
}

func TestExtractClientIP_NilHeaderFallsBackToRemoteAddr(t *testing.T) {
	ip := extractClientIP(nil, "10.0.0.1:12345", true)
	assert.Equal(t, "10.0.0.1", ip)
}

func TestExtractClientIP_InvalidRemoteAddr(t *testing.T) {
	ip := extractClientIP(nil, "invalid-remote", false)
	assert.Equal(t, "", ip)
}
