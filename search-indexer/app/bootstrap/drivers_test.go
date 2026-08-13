package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"search-indexer/config"
	"search-indexer/driver"
	"search-indexer/gateway"

	"github.com/meilisearch/meilisearch-go"
)

// stallCeiling is how long a test waits before declaring a call unbounded.
// Generous relative to the timeouts under test so a loaded CI box does not
// flake, short enough that a genuine hang fails the run.
const stallCeiling = 10 * time.Second

// newStallingMeilisearchServer returns a server that completes the TCP
// handshake and then never answers. This is the shape a Meilisearch whose
// task queue is saturated presents to its clients, and the one meilisearch-go
// cannot survive on its own: baseTransport's 30s budget is the net.Dialer's
// dial timeout, which this server passes, and the default http.Client sets no
// response timeout at all.
func newStallingMeilisearchServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	// t.Cleanup is LIFO: release the handler before Close, which otherwise
	// blocks forever on the in-flight request.
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return srv
}

// setMeiliTimeout shrinks (or stretches) the configured Meilisearch timeout
// for one test so bounded-ness can be observed without waiting out the 15s
// default.
func setMeiliTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := config.MeiliTimeout
	config.MeiliTimeout = d
	t.Cleanup(func() { config.MeiliTimeout = prev })
}

// TestMeilisearchHTTPClient_HasResponseTimeoutAndKeepsTransportDefaults pins
// the root fix: every meilisearch-go call must run on a client with a
// response timeout, without losing the pooling and dial settings the SDK's
// own baseTransport installs.
func TestMeilisearchHTTPClient_HasResponseTimeoutAndKeepsTransportDefaults(t *testing.T) {
	setMeiliTimeout(t, 7*time.Second)

	c := meilisearchHTTPClient()

	if c.Timeout != config.MeiliTimeout {
		t.Fatalf("client Timeout = %v, want config.MeiliTimeout (%v)", c.Timeout, config.MeiliTimeout)
	}

	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client Transport = %T, want *http.Transport", c.Transport)
	}
	if tr.Proxy == nil {
		t.Error("Transport.Proxy is nil; the SDK's baseTransport honours the proxy environment")
	}
	if tr.DialContext == nil {
		t.Error("Transport.DialContext is nil; the SDK's baseTransport sets a dial timeout and keep-alive there")
	}
	if tr.MaxIdleConns == 0 || tr.MaxIdleConnsPerHost != tr.MaxIdleConns {
		t.Errorf("MaxIdleConns = %d, MaxIdleConnsPerHost = %d; the SDK's baseTransport pins both to the same pool size",
			tr.MaxIdleConns, tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout == 0 || tr.TLSHandshakeTimeout == 0 || tr.ExpectContinueTimeout == 0 {
		t.Errorf("IdleConnTimeout = %v, TLSHandshakeTimeout = %v, ExpectContinueTimeout = %v; all three are set by the SDK's baseTransport",
			tr.IdleConnTimeout, tr.TLSHandshakeTimeout, tr.ExpectContinueTimeout)
	}
}

// TestMeilisearchHTTPClient_BoundsNonContextSDKCalls covers the calls no
// context can rescue: meilisearch-go's non-context methods bake
// context.Background() in, so only an http.Client timeout can end them.
func TestMeilisearchHTTPClient_BoundsNonContextSDKCalls(t *testing.T) {
	setMeiliTimeout(t, 200*time.Millisecond)
	srv := newStallingMeilisearchServer(t)

	m := meilisearch.New(srv.URL, meilisearch.WithCustomClient(meilisearchHTTPClient()))

	done := make(chan error, 1)
	go func() {
		_, err := m.Health()
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected Health() to fail against a server that never answers")
		}
	case <-time.After(stallCeiling):
		t.Fatal("Health() never returned: the SDK's default http.Client sets no response timeout")
	}
}

// TestInitMeilisearchClients_ReturnsWhenMeilisearchNeverAnswers is the
// startup failure this fix exists for: the health probe runs before any
// listener is created, so an unbounded one means :9300 and :9301 never bind
// and the container is a black hole rather than a failing one.
func TestInitMeilisearchClients_ReturnsWhenMeilisearchNeverAnswers(t *testing.T) {
	setMeiliTimeout(t, 200*time.Millisecond)
	srv := newStallingMeilisearchServer(t)
	t.Setenv("MEILISEARCH_HOST", srv.URL)
	t.Setenv("MEILISEARCH_API_KEY", "test-key")
	t.Setenv("MEILISEARCH_SEARCH_API_KEY", "")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, _, err := initMeilisearchClients(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected initMeilisearchClients to fail against a Meilisearch that never answers")
		}
	case <-time.After(stallCeiling):
		t.Fatal("initMeilisearchClients never returned: the startup health probe is unbounded")
	}
}

// TestInitMeilisearchClients_HealthProbeHonoursContext pins the probe to the
// caller's context as well. A response timeout alone still holds SIGTERM for
// a full MEILI_TIMEOUT per attempt; the non-context Health() holds it forever.
func TestInitMeilisearchClients_HealthProbeHonoursContext(t *testing.T) {
	setMeiliTimeout(t, 30*time.Second)
	srv := newStallingMeilisearchServer(t)
	t.Setenv("MEILISEARCH_HOST", srv.URL)
	t.Setenv("MEILISEARCH_API_KEY", "test-key")
	t.Setenv("MEILISEARCH_SEARCH_API_KEY", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, _, err := initMeilisearchClients(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected initMeilisearchClients to fail when its context is already done")
		}
	case <-time.After(stallCeiling):
		t.Fatal("initMeilisearchClients ignored the cancelled context: the health probe does not take one")
	}
}

// TestEnsureIndex_ReturnsWhenMeilisearchNeverAnswers covers the second
// startup call that precedes the listeners. Run passes the signal context,
// which carries no deadline, so EnsureIndex has to be bounded without one.
func TestEnsureIndex_ReturnsWhenMeilisearchNeverAnswers(t *testing.T) {
	setMeiliTimeout(t, 200*time.Millisecond)
	srv := newStallingMeilisearchServer(t)

	client := meilisearch.New(srv.URL, meilisearch.WithCustomClient(meilisearchHTTPClient()))
	searchEngine := gateway.NewSearchEngineGateway(
		driver.NewMeilisearchDriverWithClients(client, nil, "articles"),
	)

	done := make(chan error, 1)
	go func() {
		done <- searchEngine.EnsureIndex(context.Background())
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected EnsureIndex to fail against a Meilisearch that never answers")
		}
	case <-time.After(stallCeiling):
		t.Fatal("EnsureIndex never returned: it runs before the HTTP listeners open, so :9300/:9301 never bind")
	}
}
