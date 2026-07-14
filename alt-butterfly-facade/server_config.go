package main

import (
	"context"
	"log/slog"

	"alt-butterfly-facade/config"
	"alt-butterfly-facade/internal/handler"
	"alt-butterfly-facade/internal/server"
)

// buildServerConfig converts the loaded application config plus the
// resolved backend URLs/secret into the server.Config that main() hands to
// server.NewServerWithTransports. It exists as its own function so that
// mapping can be exercised by main_test.go: main() itself blocks on
// signal.Notify and os.Exit()s on every failure path, so it can't be driven
// from a normal unit test.
//
// cfg.EnableCache / EnableCircuitBreaker / EnableDedup /
// EnableErrorNormalization (and their tuning fields) must reach
// server.Config.BFFConfig, or server.go's feature switch
// (internal/server/server.go) silently falls back to the legacy
// ProxyHandler regardless of the configured flags. See
// TestBuildServerConfig_WiresBFFConfigFromAppConfig and
// TestBuildServerConfig_ResultingServer_UsesBFFHandler in main_test.go.
func buildServerConfig(cfg *config.Config, backendURL, ttsURL, acolyteURL string, secret []byte) server.Config {
	return server.Config{
		BackendURL:        backendURL,
		BackendRESTURL:    cfg.BackendRESTURL,
		Secret:            secret,
		Issuer:            cfg.BackendTokenIssuer,
		Audience:          cfg.BackendTokenAudience,
		RequestTimeout:    cfg.RequestTimeout,
		StreamingTimeout:  cfg.StreamingTimeout,
		TTSConnectURL:     ttsURL,
		AcolyteConnectURL: acolyteURL,
		BFFConfig: handler.BFFConfig{
			EnableCache:              cfg.EnableCache,
			EnableCircuitBreaker:     cfg.EnableCircuitBreaker,
			EnableDedup:              cfg.EnableDedup,
			EnableErrorNormalization: cfg.EnableErrorNormalization,
			CacheMaxSize:             cfg.CacheMaxSize,
			CacheDefaultTTL:          cfg.CacheDefaultTTL,
			CBFailureThreshold:       cfg.CBFailureThreshold,
			CBSuccessThreshold:       cfg.CBSuccessThreshold,
			CBOpenTimeout:            cfg.CBOpenTimeout,
			DedupWindow:              cfg.DedupWindow,
		},
	}
}

// logBFFFeatureWiring logs the wiring state of each BFF feature flag at
// startup. Per CLAUDE.md Rule 8 (no silent fallback for unwired
// dependencies), "enabled" vs "disabled" must be loud and explicit so a
// forgotten wire (this function never being called) is distinguishable from
// an intentional config choice, instead of silently falling back to the
// legacy ProxyHandler behavior in server.go's feature switch.
func logBFFFeatureWiring(ctx context.Context, cfg *config.Config) {
	slog.InfoContext(ctx, "bff.cache.wiring", "enabled", cfg.EnableCache)
	slog.InfoContext(ctx, "bff.circuit_breaker.wiring", "enabled", cfg.EnableCircuitBreaker)
	slog.InfoContext(ctx, "bff.dedup.wiring", "enabled", cfg.EnableDedup)
	slog.InfoContext(ctx, "bff.error_normalization.wiring", "enabled", cfg.EnableErrorNormalization)
}
