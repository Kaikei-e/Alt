package rest

import (
	"alt/di"
	"alt/domain"
	image_fetch_port "alt/port/image_fetch_port"
	"alt/port/image_proxy_port"
	"alt/usecase/image_proxy_usecase"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stubs ---

type stubSigner struct {
	verifyURL string
	verifyErr error
}

func (s *stubSigner) GenerateProxyURL(imageURL string) string {
	return "/v1/images/proxy/sig/" + imageURL
}

func (s *stubSigner) VerifyAndDecode(_, _ string) (string, error) {
	return s.verifyURL, s.verifyErr
}

type stubCache struct {
	entry *domain.ImageProxyCacheEntry
	err   error
}

func (s *stubCache) GetCachedImage(_ context.Context, _ string) (*domain.ImageProxyCacheEntry, error) {
	return s.entry, s.err
}

func (s *stubCache) SaveCachedImage(_ context.Context, _ *domain.ImageProxyCacheEntry) error {
	return nil
}

func (s *stubCache) CleanupExpiredImages(_ context.Context) (int64, error) {
	return 0, nil
}

type stubProcessing struct {
	result *domain.ImageProxyResult
	err    error
}

func (s *stubProcessing) ProcessImage(_ context.Context, _ []byte, _ string, _ int, _ int) (*domain.ImageProxyResult, error) {
	return s.result, s.err
}

type stubDomain struct {
	allowed bool
	err     error
}

func (s *stubDomain) IsAllowedImageDomain(_ context.Context, _ string) (bool, error) {
	return s.allowed, s.err
}

type stubFetcher struct {
	result *domain.ImageFetchResult
	err    error
}

func (s *stubFetcher) FetchImage(_ context.Context, _ *url.URL, _ *domain.ImageFetchOptions) (*domain.ImageFetchResult, error) {
	return s.result, s.err
}

func newTestContainer(
	signer image_proxy_port.ImageProxySignerPort,
	cache image_proxy_port.ImageProxyCachePort,
	processing image_proxy_port.ImageProcessingPort,
	dynamicDomain image_proxy_port.DynamicDomainPort,
	fetcher image_fetch_port.ImageFetchPort,
) *di.ApplicationComponents {
	uc := image_proxy_usecase.NewImageProxyUsecase(
		fetcher, processing, cache, signer, dynamicDomain,
		nil, 600, 80, 1440,
	)
	return &di.ApplicationComponents{ImageProxyUsecase: uc}
}

func TestHandleImageProxy_DeadlineExceeded_Returns504(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/images/proxy/sig/url", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("sig", "url")
	c.SetParamValues("testsig", "dGVzdA")

	// Use a fetcher that returns DeadlineExceeded to simulate timeout
	container := newTestContainer(
		&stubSigner{verifyURL: "https://example.com/image.jpg"},
		&stubCache{},
		&stubProcessing{},
		&stubDomain{allowed: true},
		&stubFetcher{err: fmt.Errorf("fetch image: %w", context.DeadlineExceeded)},
	)

	handler := handleImageProxy(container)
	require.NoError(t, handler(c))

	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
	assert.Empty(t, rec.Body.Bytes(), "should return no content body")
}

func TestHandleImageProxy_FetchError_Returns502(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/images/proxy/sig/url", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("sig", "url")
	c.SetParamValues("testsig", "dGVzdA")

	container := newTestContainer(
		&stubSigner{verifyURL: "https://example.com/image.jpg"},
		&stubCache{},
		&stubProcessing{},
		&stubDomain{allowed: true},
		&stubFetcher{err: fmt.Errorf("connection refused")},
	)

	handler := handleImageProxy(container)
	require.NoError(t, handler(c))

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Empty(t, rec.Body.Bytes(), "should return no content body")
}

func TestHandleImageProxy_SignatureError_Returns403(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/images/proxy/sig/url", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("sig", "url")
	c.SetParamValues("badsig", "dGVzdA")

	container := newTestContainer(
		&stubSigner{verifyErr: fmt.Errorf("invalid")},
		&stubCache{},
		&stubProcessing{},
		&stubDomain{allowed: true},
		&stubFetcher{},
	)

	handler := handleImageProxy(container)
	require.NoError(t, handler(c))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleImageProxy_RateLimitError_Returns429(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/images/proxy/sig/url", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("sig", "url")
	c.SetParamValues("testsig", "dGVzdA")

	container := newTestContainer(
		&stubSigner{verifyURL: "https://cdn.example.com/image.jpg"},
		&stubCache{},
		&stubProcessing{},
		&stubDomain{allowed: true},
		&stubFetcher{err: fmt.Errorf("rate limit: Wait(n=1) would exceed context deadline")},
	)

	handler := handleImageProxy(container)
	require.NoError(t, handler(c))

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Empty(t, rec.Body.Bytes(), "should return no content body")
}

func TestHandleImageProxy_CacheControl_7Day(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/images/proxy/sig/url", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("sig", "url")
	c.SetParamValues("testsig", "dGVzdA")

	container := newTestContainer(
		&stubSigner{verifyURL: "https://example.com/image.jpg"},
		&stubCache{},
		&stubProcessing{result: &domain.ImageProxyResult{
			Data:        []byte("fake-image"),
			ContentType: "image/jpeg",
			ETag:        "abc123",
		}},
		&stubDomain{allowed: true},
		&stubFetcher{result: &domain.ImageFetchResult{
			Data:        []byte("raw"),
			ContentType: "image/jpeg",
		}},
	)

	handler := handleImageProxy(container)
	require.NoError(t, handler(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "public, max-age=604800, immutable", rec.Header().Get("Cache-Control"))
}
