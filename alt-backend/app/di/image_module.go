package di

import (
	"alt/orchestrator/gateway/image_fetch_gateway"
	"alt/orchestrator/gateway/image_proxy_gateway"
	"alt/orchestrator/usecase/image_fetch_usecase"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/utils/image_proxy"
	"alt/utils/rate_limiter"
	"log/slog"
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
	imageProxyWired := logImageProxyWiringState(cfg.ImageProxy.Enabled, cfg.ImageProxy.Secret != "")
	if imageProxyWired {
		imageProxySigner := image_proxy.NewSigner(cfg.ImageProxy.Secret)
		// The cache tier is alt-data-hub's since ADR-000954 Wave 3
		// (catalog §2.E). Fetching, resizing and re-encoding the image stay
		// here — outbound HTTP and a CPU-bound transform, neither of which
		// D4 moves.
		imageProxyCacheGw := infra.ImageProxyCacheGateway
		imageProxyProcessingGw := image_proxy_gateway.NewProcessingGateway()
		imageProxyDynamicDomainGw := image_proxy_gateway.NewDynamicDomainGateway(infra.FeedLinkGateway)
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

// logImageProxyWiringState emits the loud enabled/disabled/misconfigured
// signal CLAUDE.md rule 8 / .claude/rules/di-wiring.md require. Without it,
// cfg.ImageProxy.Enabled=true with an empty Secret (e.g. Docker secret file
// missing/unreadable) silently left ImageProxyUsecaseInstance nil with no
// startup log — indistinguishable from a deliberate
// IMAGE_PROXY_ENABLED=false, and every warmer/backfill job downstream just
// logged "not configured" at Info level forever (findings [6], [12]).
// A misconfiguration (enabled but no secret) is a startup failure rather than
// a limp-mode warning, in every environment. config.validateImageProxyConfig
// already rejects it before the container is built, so reaching this branch
// means the config guard was bypassed — rule 8 says panic rather than return a
// nil usecase. This used to be keyed on appEnv == "production", which is set
// in no compose file and therefore never fired.
func logImageProxyWiringState(enabled, hasSecret bool) bool {
	switch {
	case enabled && hasSecret:
		slog.Info("image_proxy_enabled")
		return true
	case enabled && !hasSecret:
		slog.Error("image_proxy_misconfigured", "reason", "IMAGE_PROXY_ENABLED=true but secret is empty")
		panic("IMAGE_PROXY_ENABLED=true requires a non-empty secret — refusing to start with image proxy silently disabled")
	default:
		slog.Warn("image_proxy_disabled", "reason", "IMAGE_PROXY_ENABLED=false")
		return false
	}
}
