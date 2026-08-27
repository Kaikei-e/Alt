package og_resolve_gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"alt/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The request must identify us honestly and carry a Referer.
//
// The Referer is what makes this request legible to the origin as what it
// actually is — a reader looking at that page — rather than an anonymous
// crawler, and it is the header most hotlink and bot rules key on. The
// User-Agent stays a truthful Alt string: the point of resolving on demand is
// that we no longer need to be mistaken for a browser to be welcome.
func TestFetchOgImage_SendsHonestUserAgentAndReferer(t *testing.T) {
	var gotUA, gotReferer, gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		gotUA = r.Header.Get("User-Agent")
		gotReferer = r.Header.Get("Referer")
		gotAccept = r.Header.Get("Accept")
		_, _ = w.Write([]byte(`<html><head><meta property="og:image" content="https://cdn.example.com/x.png"></head></html>`))
	}))
	defer srv.Close()

	gw := newTestGateway(srv.Client())

	img, refusal, err := gw.FetchOgImage(context.Background(), srv.URL+"/posts/hello")
	require.NoError(t, err)
	assert.Empty(t, refusal)
	assert.Equal(t, "https://cdn.example.com/x.png", img)

	assert.Contains(t, gotUA, "Alt", "the fetcher must say who it is")
	assert.NotContains(t, strings.ToLower(gotUA), "mozilla",
		"resolving on demand is what earns the request; impersonating a browser is not part of the bargain")
	assert.Equal(t, srv.URL+"/posts/hello", gotReferer)
	assert.Contains(t, gotAccept, "text/html")
}

// robots.txt is consulted before the page, and a disallow is a settled answer:
// no page request at all, and a refusal that is never retried in this window.
func TestFetchOgImage_RobotsDisallowIsHonoured(t *testing.T) {
	pageRequests := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /posts/\n"))
			return
		}
		pageRequests++
		_, _ = w.Write([]byte(`<html><head><meta property="og:image" content="https://cdn.example.com/x.png"></head></html>`))
	}))
	defer srv.Close()

	gw := newTestGateway(srv.Client())

	img, refusal, err := gw.FetchOgImage(context.Background(), srv.URL+"/posts/hello")
	require.NoError(t, err)
	assert.Empty(t, img)
	assert.Equal(t, domain.OgImageRefusedByRobots, refusal)
	assert.Zero(t, pageRequests, "a disallowed path must not be fetched at all")
	assert.Zero(t, refusal.RetryAfter(1), "a stated policy is not a transient fault")
	assert.Zero(t, refusal.RetryAfter(9),
		"and it stays a policy however many times it is heard — this is the one refusal class the escalating bar leaves alone")
}

// A path the site allows is fetched even when robots.txt restricts others.
func TestFetchOgImage_RobotsAllowsOtherPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin/\n"))
			return
		}
		_, _ = w.Write([]byte(`<html><head><meta property="og:image" content="https://cdn.example.com/ok.png"></head></html>`))
	}))
	defer srv.Close()

	gw := newTestGateway(srv.Client())

	img, refusal, err := gw.FetchOgImage(context.Background(), srv.URL+"/posts/hello")
	require.NoError(t, err)
	assert.Empty(t, refusal)
	assert.Equal(t, "https://cdn.example.com/ok.png", img)
}

// An origin that refuses maps to a typed refusal rather than an error, because
// "they said no" is an outcome to record, not a failure to retry.
func TestFetchOgImage_StatusCodesMapToRefusals(t *testing.T) {
	cases := []struct {
		status int
		want   domain.OgImageRefusal
	}{
		{http.StatusForbidden, domain.OgImageRefusedForbidden},
		{http.StatusNotFound, domain.OgImageRefusedNotFound},
		{http.StatusInternalServerError, domain.OgImageFetchError},
	}

	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(tc.status)
		}))

		gw := newTestGateway(srv.Client())
		img, refusal, err := gw.FetchOgImage(context.Background(), srv.URL+"/posts/hello")

		require.NoError(t, err, "status %d", tc.status)
		assert.Empty(t, img)
		assert.Equal(t, tc.want, refusal, "status %d", tc.status)

		srv.Close()
	}
}

// A page that simply carries no og:image is a settled answer too — re-fetching
// it on the next scroll would cost the origin a request to learn the same
// thing.
func TestFetchOgImage_NoTagIsSettled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`<html><head><title>no image here</title></head></html>`))
	}))
	defer srv.Close()

	gw := newTestGateway(srv.Client())

	img, refusal, err := gw.FetchOgImage(context.Background(), srv.URL+"/posts/hello")
	require.NoError(t, err)
	assert.Empty(t, img)
	assert.Equal(t, domain.OgImageNoTag, refusal)
	assert.Zero(t, refusal.RetryAfter(1))
	assert.Zero(t, refusal.RetryAfter(9), "a page without the tag does not grow one by being asked again")
}

// robots.txt is fetched once per host, not once per feed. A grid of twenty
// cards from one publisher must not cost that publisher twenty robots.txt
// requests on top of the pages.
func TestFetchOgImage_RobotsFetchedOncePerHost(t *testing.T) {
	robotsRequests := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			robotsRequests++
			_, _ = w.Write([]byte("User-agent: *\nDisallow: /admin/\n"))
			return
		}
		_, _ = w.Write([]byte(`<html><head><meta property="og:image" content="https://cdn.example.com/x.png"></head></html>`))
	}))
	defer srv.Close()

	gw := newTestGateway(srv.Client())

	for i := 0; i < 3; i++ {
		_, _, err := gw.FetchOgImage(context.Background(), srv.URL+"/posts/hello")
		require.NoError(t, err)
	}

	assert.Equal(t, 1, robotsRequests)
}

// passthroughValidator stands in for the SSRF gate. The real validator
// correctly refuses the loopback address httptest serves on, so these tests
// would otherwise only ever exercise the rejection path.
type passthroughValidator struct{}

func (passthroughValidator) CanonicalRequestURL(_ context.Context, u *url.URL) (string, error) {
	return u.String(), nil
}

func newTestGateway(client *http.Client) *Gateway {
	return NewGateway(nil, client, passthroughValidator{})
}
