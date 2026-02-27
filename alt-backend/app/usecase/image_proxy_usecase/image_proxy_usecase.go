package image_proxy_usecase

import (
	"alt/domain"
	"alt/port/image_proxy_port"
	"alt/utils/image_proxy"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
	"context"
	"fmt"
	"net/url"
	"time"

	image_fetch_port "alt/port/image_fetch_port"
)

// ImageProxyUsecase orchestrates image proxy operations.
type ImageProxyUsecase struct {
	imageFetchPort image_fetch_port.ImageFetchPort
	processing     image_proxy_port.ImageProcessingPort
	cache          image_proxy_port.ImageProxyCachePort
	signer         image_proxy_port.ImageProxySignerPort
	dynamicDomain  image_proxy_port.DynamicDomainPort
	rateLimiter    *rate_limiter.HostRateLimiter
	maxWidth       int
	webpQuality    int
	cacheTTL       time.Duration
}

// NewImageProxyUsecase creates a new ImageProxyUsecase.
func NewImageProxyUsecase(
	imageFetchPort image_fetch_port.ImageFetchPort,
	processing image_proxy_port.ImageProcessingPort,
	cache image_proxy_port.ImageProxyCachePort,
	signer image_proxy_port.ImageProxySignerPort,
	dynamicDomain image_proxy_port.DynamicDomainPort,
	rateLimiter *rate_limiter.HostRateLimiter,
	maxWidth int,
	webpQuality int,
	cacheTTLMinutes int,
) *ImageProxyUsecase {
	ttl := time.Duration(cacheTTLMinutes) * time.Minute
	if ttl == 0 {
		ttl = domain.ImageProxyCacheTTL
	}
	return &ImageProxyUsecase{
		imageFetchPort: imageFetchPort,
		processing:     processing,
		cache:          cache,
		signer:         signer,
		dynamicDomain:  dynamicDomain,
		rateLimiter:    rateLimiter,
		maxWidth:       maxWidth,
		webpQuality:    webpQuality,
		cacheTTL:       ttl,
	}
}

// ProxyImage serves a proxied image: verify signature, check cache, fetch+process if needed.
func (u *ImageProxyUsecase) ProxyImage(ctx context.Context, sig, encodedURL string) (*domain.ImageProxyResult, error) {
	// 1. Verify HMAC signature + decode URL
	originalURL, err := u.signer.VerifyAndDecode(sig, encodedURL)
	if err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}

	// 2. Check cache
	urlHash := image_proxy.URLHash(originalURL)
	cached, err := u.cache.GetCachedImage(ctx, urlHash)
	if err != nil {
		logger.SafeErrorContext(ctx, "cache lookup failed", "error", err)
		// Continue to fetch on cache error
	}
	if cached != nil && time.Now().Before(cached.ExpiresAt) {
		return &domain.ImageProxyResult{
			Data:        cached.Data,
			ContentType: cached.ContentType,
			Width:       cached.Width,
			Height:      cached.Height,
			SizeBytes:   cached.SizeBytes,
			ETag:        cached.ETag,
			ExpiresAt:   cached.ExpiresAt,
		}, nil
	}

	// 3. Dynamic domain check
	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		return nil, fmt.Errorf("invalid image URL: %w", err)
	}

	allowed, err := u.dynamicDomain.IsAllowedImageDomain(ctx, parsedURL.Hostname())
	if err != nil {
		return nil, fmt.Errorf("domain check failed: %w", err)
	}
	if !allowed {
		return nil, fmt.Errorf("domain not allowed: %s", parsedURL.Hostname())
	}

	// 4. Rate limit
	if u.rateLimiter != nil {
		if err := u.rateLimiter.WaitForHost(ctx, originalURL); err != nil {
			return nil, fmt.Errorf("rate limit: %w", err)
		}
	}

	// 5. Fetch image using existing SSRF-protected fetcher
	fetchResult, err := u.imageFetchPort.FetchImage(ctx, parsedURL, domain.NewImageFetchOptions())
	if err != nil {
		return nil, fmt.Errorf("fetch image: %w", err)
	}

	// 6. Process: resize + WebP encode
	processed, err := u.processing.ProcessImage(ctx, fetchResult.Data, fetchResult.ContentType, u.maxWidth, u.webpQuality)
	if err != nil {
		return nil, fmt.Errorf("process image: %w", err)
	}

	// 7. Save to cache (best effort)
	cacheEntry := &domain.ImageProxyCacheEntry{
		URLHash:     urlHash,
		OriginalURL: originalURL,
		Data:        processed.Data,
		ContentType: processed.ContentType,
		Width:       processed.Width,
		Height:      processed.Height,
		SizeBytes:   processed.SizeBytes,
		ETag:        processed.ETag,
		ExpiresAt:   time.Now().Add(u.cacheTTL),
	}
	if err := u.cache.SaveCachedImage(ctx, cacheEntry); err != nil {
		logger.SafeErrorContext(ctx, "failed to cache image", "error", err, "url", originalURL)
	}

	return processed, nil
}

// GenerateProxyURL generates an HMAC-signed proxy URL for an image.
func (u *ImageProxyUsecase) GenerateProxyURL(imageURL string) string {
	if imageURL == "" {
		return ""
	}
	return u.signer.GenerateProxyURL(imageURL)
}

// BatchGenerateProxyURLs generates proxy URLs for images from article_heads.
// This is used by the Connect-RPC BatchPrefetchImages handler.
func (u *ImageProxyUsecase) BatchGenerateProxyURLs(ctx context.Context, ogImageURLs map[string]string) map[string]string {
	result := make(map[string]string, len(ogImageURLs))
	for articleID, ogURL := range ogImageURLs {
		if ogURL != "" {
			result[articleID] = u.signer.GenerateProxyURL(ogURL)
		}
	}
	return result
}

// WarmCache prefetches and caches an image (fire-and-forget).
func (u *ImageProxyUsecase) WarmCache(ctx context.Context, imageURL string) {
	if imageURL == "" {
		return
	}

	urlHash := image_proxy.URLHash(imageURL)
	cached, _ := u.cache.GetCachedImage(ctx, urlHash)
	if cached != nil {
		return // Already cached
	}

	proxyURL := u.signer.GenerateProxyURL(imageURL)
	if proxyURL == "" {
		return
	}

	// Extract sig and encodedURL from the proxy URL path
	// Format: /v1/images/proxy/{sig}/{encodedURL}
	const prefix = "/v1/images/proxy/"
	rest := proxyURL[len(prefix):]
	for i, c := range rest {
		if c == '/' {
			sig := rest[:i]
			encoded := rest[i+1:]
			_, _ = u.ProxyImage(ctx, sig, encoded)
			return
		}
	}
}
