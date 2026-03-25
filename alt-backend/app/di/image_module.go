package di

import (
	"alt/gateway/image_fetch_gateway"
	"alt/gateway/image_proxy_gateway"
	"alt/usecase/image_fetch_usecase"
	"alt/usecase/image_proxy_usecase"
	"alt/utils/image_proxy"
	"alt/utils/rate_limiter"
	"net/http"
	"time"
)

// ImageModule holds all image-domain components.
type ImageModule struct {
	ImageFetchUsecase image_fetch_usecase.ImageFetchUsecaseInterface
	ImageProxyUsecase *image_proxy_usecase.ImageProxyUsecase
}

func newImageModule(infra *InfraModule) *ImageModule {
	cfg := infra.Config

	// Image fetch components
	imageHTTPClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	imageFetchGw := image_fetch_gateway.NewImageFetchGateway(imageHTTPClient)
	imageFetchUC := image_fetch_usecase.NewImageFetchUsecase(imageFetchGw)

	// Image proxy components
	// CDN public images are fetched on-demand per user action, not crawled.
	// 1 req/s/host is conservative enough and avoids context deadline exceeded
	// when multiple images from the same host are requested concurrently.
	imageProxyRateLimiter := rate_limiter.NewHostRateLimiter(1 * time.Second)
	var imageProxyUsecaseInstance *image_proxy_usecase.ImageProxyUsecase
	if cfg.ImageProxy.Enabled && cfg.ImageProxy.Secret != "" {
		imageProxySigner := image_proxy.NewSigner(cfg.ImageProxy.Secret)
		imageProxyCacheGw := image_proxy_gateway.NewCacheGateway(infra.AltDBRepository)
		imageProxyProcessingGw := image_proxy_gateway.NewProcessingGateway()
		imageProxyDynamicDomainGw := image_proxy_gateway.NewDynamicDomainGateway(infra.AltDBRepository)
		imageProxyUsecaseInstance = image_proxy_usecase.NewImageProxyUsecase(
			imageFetchGw,
			imageProxyProcessingGw,
			imageProxyCacheGw,
			imageProxySigner,
			imageProxyDynamicDomainGw,
			imageProxyRateLimiter,
			cfg.ImageProxy.MaxWidth,
			cfg.ImageProxy.WebPQuality,
			cfg.ImageProxy.CacheTTLMin,
		)
	}

	return &ImageModule{
		ImageFetchUsecase: imageFetchUC,
		ImageProxyUsecase: imageProxyUsecaseInstance,
	}
}
