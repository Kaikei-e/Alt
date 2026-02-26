package image_proxy_usecase

import (
	"alt/domain"
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"
)

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
	cached *domain.ImageProxyCacheEntry
	getErr error
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
