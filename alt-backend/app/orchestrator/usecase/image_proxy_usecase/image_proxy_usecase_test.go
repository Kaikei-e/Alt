package image_proxy_usecase

import (
	"alt/domain"
	"context"
	"fmt"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func atomicAddInt32(addr *int32, delta int32) int32 { return atomic.AddInt32(addr, delta) }
func atomicLoadInt32(addr *int32) int32             { return atomic.LoadInt32(addr) }

// --- Mock implementations ---

type mockImageFetchPort struct {
	result *domain.ImageFetchResult
	err    error
}

func (m *mockImageFetchPort) FetchImage(ctx context.Context, imageURL *url.URL, options *domain.ImageFetchOptions) (*domain.ImageFetchResult, error) {
	return m.result, m.err
}

type mockImageProcessingPort struct {
	result *domain.ImageProxyResult
	err    error
}

func (m *mockImageProcessingPort) ProcessImage(ctx context.Context, data []byte, contentType string, maxWidth int, quality int) (*domain.ImageProxyResult, error) {
	return m.result, m.err
}

type mockImageProxyCachePort struct {
	cached  *domain.ImageProxyCacheEntry
	getErr  error
	saveErr error
}

func (m *mockImageProxyCachePort) GetCachedImage(ctx context.Context, urlHash string) (*domain.ImageProxyCacheEntry, error) {
	return m.cached, m.getErr
}

func (m *mockImageProxyCachePort) SaveCachedImage(ctx context.Context, entry *domain.ImageProxyCacheEntry) error {
	return m.saveErr
}

func (m *mockImageProxyCachePort) CleanupExpiredImages(ctx context.Context) (int64, error) {
	return 0, nil
}

type mockSignerPort struct {
	proxyURL   string
	decodedURL string
	verifyErr  error
}

func (m *mockSignerPort) GenerateProxyURL(imageURL string) string {
	return m.proxyURL
}

func (m *mockSignerPort) VerifyAndDecode(signature, encodedURL string) (string, error) {
	return m.decodedURL, m.verifyErr
}

type mockDynamicDomainPort struct {
	allowed bool
	err     error
}

func (m *mockDynamicDomainPort) IsAllowedImageDomain(ctx context.Context, hostname string) (bool, error) {
	return m.allowed, m.err
}

type mockRateLimiterPort struct {
	waitForURLCalled bool
	lastURL          string
	err              error
}

func (m *mockRateLimiterPort) WaitForHost(ctx context.Context, host string) error {
	return m.err
}

func (m *mockRateLimiterPort) WaitForURL(ctx context.Context, url string) error {
	m.waitForURLCalled = true
	m.lastURL = url
	return m.err
}

func (m *mockRateLimiterPort) GetRemainingRequests(host string) int {
	return 1
}

func (m *mockRateLimiterPort) GetNextAvailableTime(host string) time.Time {
	return time.Now()
}

// --- Tests ---

func TestProxyImage_CacheHit(t *testing.T) {
	cached := &domain.ImageProxyCacheEntry{
		Data:        []byte("cached-webp-data"),
		ContentType: "image/webp",
		Width:       600,
		Height:      300,
		SizeBytes:   16,
		ETag:        "abc123",
		ExpiresAt:   time.Now().Add(time.Hour),
	}

	uc := NewImageProxyUsecase(
		&mockImageFetchPort{},
		&mockImageProcessingPort{},
		&mockImageProxyCachePort{cached: cached},
		&mockSignerPort{decodedURL: "https://example.com/img.jpg"},
		&mockDynamicDomainPort{allowed: true},
		nil, 600, 80, 720,
	)

	result, err := uc.ProxyImage(context.Background(), "valid-sig", "encoded-url")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Data) != "cached-webp-data" {
		t.Error("expected cached data")
	}
	if result.ContentType != "image/webp" {
		t.Errorf("expected image/webp, got %s", result.ContentType)
	}
}

func TestProxyImage_CacheMiss_FetchAndProcess(t *testing.T) {
	processedResult := &domain.ImageProxyResult{
		Data:        []byte("processed-webp"),
		ContentType: "image/webp",
		Width:       600,
		Height:      300,
		SizeBytes:   14,
		ETag:        "new-etag",
		ExpiresAt:   time.Now().Add(12 * time.Hour),
	}

	uc := NewImageProxyUsecase(
		&mockImageFetchPort{result: &domain.ImageFetchResult{
			Data:        []byte("raw-jpeg-data"),
			ContentType: "image/jpeg",
			Size:        13,
		}},
		&mockImageProcessingPort{result: processedResult},
		&mockImageProxyCachePort{cached: nil},
		&mockSignerPort{decodedURL: "https://example.com/img.jpg"},
		&mockDynamicDomainPort{allowed: true},
		nil, 600, 80, 720,
	)

	result, err := uc.ProxyImage(context.Background(), "valid-sig", "encoded-url")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Data) != "processed-webp" {
		t.Error("expected processed data")
	}
}

func TestProxyImage_InvalidSignature(t *testing.T) {
	uc := NewImageProxyUsecase(
		&mockImageFetchPort{},
		&mockImageProcessingPort{},
		&mockImageProxyCachePort{},
		&mockSignerPort{verifyErr: fmt.Errorf("invalid signature")},
		&mockDynamicDomainPort{},
		nil, 600, 80, 720,
	)

	_, err := uc.ProxyImage(context.Background(), "bad-sig", "encoded-url")
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestProxyImage_DomainNotAllowed(t *testing.T) {
	uc := NewImageProxyUsecase(
		&mockImageFetchPort{},
		&mockImageProcessingPort{},
		&mockImageProxyCachePort{cached: nil},
		&mockSignerPort{decodedURL: "https://evil.com/img.jpg"},
		&mockDynamicDomainPort{allowed: false},
		nil, 600, 80, 720,
	)

	_, err := uc.ProxyImage(context.Background(), "valid-sig", "encoded-url")
	if err == nil {
		t.Fatal("expected error for disallowed domain")
	}
}

func TestGenerateProxyURL(t *testing.T) {
	uc := NewImageProxyUsecase(
		&mockImageFetchPort{},
		&mockImageProcessingPort{},
		&mockImageProxyCachePort{},
		&mockSignerPort{proxyURL: "/v1/images/proxy/abc/def"},
		&mockDynamicDomainPort{},
		nil, 600, 80, 720,
	)

	result := uc.GenerateProxyURL("https://example.com/img.jpg")
	if result != "/v1/images/proxy/abc/def" {
		t.Errorf("unexpected proxy URL: %s", result)
	}
}

func TestGenerateProxyURL_Empty(t *testing.T) {
	uc := NewImageProxyUsecase(
		&mockImageFetchPort{},
		&mockImageProcessingPort{},
		&mockImageProxyCachePort{},
		&mockSignerPort{},
		&mockDynamicDomainPort{},
		nil, 600, 80, 720,
	)

	result := uc.GenerateProxyURL("")
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

func TestProxyImage_WithRateLimiter(t *testing.T) {
	processedResult := &domain.ImageProxyResult{
		Data:        []byte("processed-webp"),
		ContentType: "image/webp",
		Width:       600,
		Height:      300,
		SizeBytes:   14,
		ETag:        "new-etag",
		ExpiresAt:   time.Now().Add(12 * time.Hour),
	}

	rl := &mockRateLimiterPort{}

	uc := NewImageProxyUsecase(
		&mockImageFetchPort{result: &domain.ImageFetchResult{
			Data:        []byte("raw-jpeg-data"),
			ContentType: "image/jpeg",
			Size:        13,
		}},
		&mockImageProcessingPort{result: processedResult},
		&mockImageProxyCachePort{cached: nil},
		&mockSignerPort{decodedURL: "https://example.com/img.jpg"},
		&mockDynamicDomainPort{allowed: true},
		rl, 600, 80, 720,
	)

	result, err := uc.ProxyImage(context.Background(), "valid-sig", "encoded-url")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Data) != "processed-webp" {
		t.Error("expected processed data")
	}
	if !rl.waitForURLCalled {
		t.Error("expected WaitForURL to be called")
	}
	if rl.lastURL != "https://example.com/img.jpg" {
		t.Errorf("expected url https://example.com/img.jpg, got %s", rl.lastURL)
	}
}

func TestBatchGenerateProxyURLs(t *testing.T) {
	uc := NewImageProxyUsecase(
		&mockImageFetchPort{},
		&mockImageProcessingPort{},
		&mockImageProxyCachePort{},
		&mockSignerPort{proxyURL: "/v1/images/proxy/sig/url"},
		&mockDynamicDomainPort{},
		nil, 600, 80, 720,
	)

	ogURLs := map[string]string{
		"article-1": "https://example.com/img1.jpg",
		"article-2": "",
		"article-3": "https://example.com/img3.jpg",
	}

	result := uc.BatchGenerateProxyURLs(context.Background(), ogURLs)
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
	if _, ok := result["article-1"]; !ok {
		t.Error("expected article-1 in results")
	}
	if _, ok := result["article-2"]; ok {
		t.Error("article-2 should not be in results (empty URL)")
	}
}

func TestParseProxyURLPath(t *testing.T) {
	tests := []struct {
		name        string
		proxyURL    string
		wantSig     string
		wantEncoded string
		wantOK      bool
	}{
		{
			name:        "valid proxy URL",
			proxyURL:    "/v1/images/proxy/test-sig/aHR0cHM6Ly9leGFtcGxlLmNvbQ==",
			wantSig:     "test-sig",
			wantEncoded: "aHR0cHM6Ly9leGFtcGxlLmNvbQ==",
			wantOK:      true,
		},
		{
			name:        "valid proxy URL with slash in encoded part",
			proxyURL:    "/v1/images/proxy/sig123/sub/path/data",
			wantSig:     "sig123",
			wantEncoded: "sub/path/data",
			wantOK:      true,
		},
		{
			name:        "missing prefix",
			proxyURL:    "/api/proxy/sig123/encoded",
			wantSig:     "",
			wantEncoded: "",
			wantOK:      false,
		},
		{
			name:        "empty string",
			proxyURL:    "",
			wantSig:     "",
			wantEncoded: "",
			wantOK:      false,
		},
		{
			name:        "prefix only without slash separator",
			proxyURL:    "/v1/images/proxy/",
			wantSig:     "",
			wantEncoded: "",
			wantOK:      false,
		},
		{
			name:        "prefix with sig but no slash separator",
			proxyURL:    "/v1/images/proxy/onlysig",
			wantSig:     "",
			wantEncoded: "",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSig, gotEncoded, gotOK := parseProxyURLPath(tt.proxyURL)
			if gotOK != tt.wantOK {
				t.Fatalf("parseProxyURLPath(%q) ok = %v, want %v", tt.proxyURL, gotOK, tt.wantOK)
			}
			if gotSig != tt.wantSig {
				t.Errorf("parseProxyURLPath(%q) sig = %q, want %q", tt.proxyURL, gotSig, tt.wantSig)
			}
			if gotEncoded != tt.wantEncoded {
				t.Errorf("parseProxyURLPath(%q) encoded = %q, want %q", tt.proxyURL, gotEncoded, tt.wantEncoded)
			}
		})
	}
}

type countingFetchPort struct {
	calls   int32
	started chan struct{}
	release chan struct{}
	result  *domain.ImageFetchResult
}

func (f *countingFetchPort) FetchImage(ctx context.Context, _ *url.URL, _ *domain.ImageFetchOptions) (*domain.ImageFetchResult, error) {
	if atomicAddInt32(&f.calls, 1) == 1 && f.started != nil {
		close(f.started)
	}
	if f.release != nil {
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.result, nil
}

type countingCachePort struct {
	getCalls  int32
	saveCalls int32
}

func (c *countingCachePort) GetCachedImage(_ context.Context, _ string) (*domain.ImageProxyCacheEntry, error) {
	atomicAddInt32(&c.getCalls, 1)
	return nil, nil
}

func (c *countingCachePort) SaveCachedImage(_ context.Context, _ *domain.ImageProxyCacheEntry) error {
	atomicAddInt32(&c.saveCalls, 1)
	return nil
}

func (c *countingCachePort) CleanupExpiredImages(_ context.Context) (int64, error) {
	return 0, nil
}

type countingRateLimiterPort struct {
	calls int32
}

func (r *countingRateLimiterPort) WaitForHost(_ context.Context, _ string) error { return nil }
func (r *countingRateLimiterPort) WaitForURL(_ context.Context, _ string) error {
	atomicAddInt32(&r.calls, 1)
	return nil
}
func (r *countingRateLimiterPort) GetRemainingRequests(_ string) int       { return 1 }
func (r *countingRateLimiterPort) GetNextAvailableTime(_ string) time.Time { return time.Now() }

func TestProxyImage_SingleflightDeduplicatesConcurrentMisses(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fetchPort := &countingFetchPort{
		started: started,
		release: release,
		result: &domain.ImageFetchResult{
			Data:        []byte("raw-jpeg-data"),
			ContentType: "image/jpeg",
			Size:        13,
		},
	}
	cachePort := &countingCachePort{}
	rlPort := &countingRateLimiterPort{}
	processedResult := &domain.ImageProxyResult{
		Data:        []byte("processed-webp"),
		ContentType: "image/webp",
		Width:       600,
		Height:      300,
		SizeBytes:   14,
		ETag:        "new-etag",
		ExpiresAt:   time.Now().Add(12 * time.Hour),
	}

	uc := NewImageProxyUsecase(
		fetchPort,
		&mockImageProcessingPort{result: processedResult},
		cachePort,
		&mockSignerPort{decodedURL: "https://example.com/shared-og.jpg"},
		&mockDynamicDomainPort{allowed: true},
		rlPort, 600, 80, 720,
	)

	const callers = 4
	results := make(chan *domain.ImageProxyResult, callers)
	errs := make(chan error, callers)

	// Start first caller (e.g. WarmCache) and wait until it enters FetchImage.
	go func() {
		res, err := uc.ProxyImage(context.Background(), "valid-sig", "encoded-url")
		results <- res
		errs <- err
	}()
	<-started

	// Start remaining concurrent callers (e.g. browser <img src> requests for the same URL).
	for i := 1; i < callers; i++ {
		go func() {
			res, err := uc.ProxyImage(context.Background(), "valid-sig", "encoded-url")
			results <- res
			errs <- err
		}()
	}
	// Give concurrent callers a moment to join the in-flight singleflight group.
	time.Sleep(20 * time.Millisecond)
	close(release)

	for i := 0; i < callers; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("caller %d unexpected error: %v", i, err)
		}
		res := <-results
		if res == nil || string(res.Data) != "processed-webp" {
			t.Fatalf("caller %d unexpected result: %+v", i, res)
		}
	}

	if got := atomicLoadInt32(&fetchPort.calls); got != 1 {
		t.Errorf("FetchImage calls = %d, want 1 (concurrent misses for same URL must be coalesced)", got)
	}
	if got := atomicLoadInt32(&rlPort.calls); got != 1 {
		t.Errorf("WaitForURL calls = %d, want 1 (WarmCache and browser request must not double-consume host rate limiter)", got)
	}
	if got := atomicLoadInt32(&cachePort.saveCalls); got != 1 {
		t.Errorf("SaveCachedImage calls = %d, want 1 (concurrent misses must not race on INSERT ON CONFLICT DO UPDATE)", got)
	}
}

func TestProxyImage_SingleflightCallerCancelDoesNotAbortPeer(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fetchPort := &countingFetchPort{
		started: started,
		release: release,
		result: &domain.ImageFetchResult{
			Data:        []byte("raw-jpeg-data"),
			ContentType: "image/jpeg",
			Size:        13,
		},
	}
	cachePort := &countingCachePort{}
	processedResult := &domain.ImageProxyResult{
		Data:        []byte("processed-webp"),
		ContentType: "image/webp",
		Width:       600,
		Height:      300,
		SizeBytes:   14,
		ETag:        "new-etag",
		ExpiresAt:   time.Now().Add(12 * time.Hour),
	}

	uc := NewImageProxyUsecase(
		fetchPort,
		&mockImageProcessingPort{result: processedResult},
		cachePort,
		&mockSignerPort{decodedURL: "https://example.com/shared-og-cancel.jpg"},
		&mockDynamicDomainPort{allowed: true},
		nil, 600, 80, 720,
	)

	ctx1, cancel1 := context.WithCancel(context.Background())
	err1Ch := make(chan error, 1)
	go func() {
		_, err := uc.ProxyImage(ctx1, "valid-sig", "encoded-url")
		err1Ch <- err
	}()
	<-started

	// Second caller joins with an active context.
	res2Ch := make(chan *domain.ImageProxyResult, 1)
	err2Ch := make(chan error, 1)
	go func() {
		res, err := uc.ProxyImage(context.Background(), "valid-sig", "encoded-url")
		res2Ch <- res
		err2Ch <- err
	}()
	time.Sleep(20 * time.Millisecond)

	// Cancel the first caller while FetchImage is still in flight.
	cancel1()
	if err1 := <-err1Ch; err1 == nil {
		t.Fatal("expected cancelled caller to return context error")
	}

	// Release FetchImage; the second caller must still succeed.
	close(release)
	if err2 := <-err2Ch; err2 != nil {
		t.Fatalf("second caller failed after first caller cancelled: %v", err2)
	}
	if res2 := <-res2Ch; res2 == nil || string(res2.Data) != "processed-webp" {
		t.Fatalf("unexpected second caller result: %+v", res2)
	}
	if got := atomicLoadInt32(&fetchPort.calls); got != 1 {
		t.Errorf("FetchImage calls = %d, want 1", got)
	}
}
