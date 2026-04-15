package utils

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// MED-1: every client handed out by HTTPClientManager must have a transport
// with explicit layered timeouts. Relying on http.Client.Timeout alone lets
// slowloris peers pin connection slots indefinitely at the header-read phase.
// Exception: the summary client disables ResponseHeaderTimeout because the
// news-creator Map-Reduce pipeline does not flush response headers until the
// full multi-minute job completes; the caller always wraps the request in
// context.WithTimeout(NEWS_CREATOR_TIMEOUT), which bounds the header-read
// phase end-to-end and replaces the transport-level cap.
func TestHTTPClientManager_HardenedTransportFields(t *testing.T) {
	m := NewHTTPClientManager()

	cases := []struct {
		name                   string
		client                 *http.Client
		allowDisabledHeaderTOT bool
	}{
		{"default", m.GetDefaultClient(), false},
		{"summary", m.GetSummaryClient(), true},
		{"feed", m.GetFeedClient(), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ot, ok := c.client.Transport.(*optimizedTransport)
			require.True(t, ok, "transport must be *optimizedTransport")
			require.NotNil(t, ot.Transport)

			require.NotNil(t, ot.Transport.DialContext,
				"DialContext must be set so the dial phase has an explicit timeout")
			require.Greater(t, ot.Transport.TLSHandshakeTimeout, time.Duration(0),
				"TLSHandshakeTimeout must be positive")
			require.LessOrEqual(t, ot.Transport.TLSHandshakeTimeout, 10*time.Second,
				"TLSHandshakeTimeout must be aggressive (≤10s)")
			if c.allowDisabledHeaderTOT {
				require.Equal(t, time.Duration(0), ot.Transport.ResponseHeaderTimeout,
					"summary client must disable ResponseHeaderTimeout and rely on context timeout")
			} else {
				require.Greater(t, ot.Transport.ResponseHeaderTimeout, time.Duration(0),
					"ResponseHeaderTimeout must be set to cap slow-header attacks")
				require.LessOrEqual(t, ot.Transport.ResponseHeaderTimeout, 30*time.Second,
					"ResponseHeaderTimeout must be aggressive (≤30s)")
			}
			require.Greater(t, ot.MaxIdleConnsPerHost, 0)
			require.Greater(t, ot.IdleConnTimeout, time.Duration(0))
		})
	}
}
