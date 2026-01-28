// Package proxy provides unified proxy configuration and URL conversion utilities
// for routing HTTP requests through various proxy modes (Sidecar, Envoy, Nginx).
package proxy

// Mode represents different proxy operation modes
type Mode string

const (
	// ModeSidecar routes requests through a sidecar proxy (highest priority)
	ModeSidecar Mode = "sidecar"
	// ModeEnvoy routes requests through Envoy Dynamic Forward Proxy
	ModeEnvoy Mode = "envoy"
	// ModeNginx routes requests through nginx-external (RSS-only, lowest priority)
	ModeNginx Mode = "nginx"
	// ModeDisabled indicates no proxy is configured
	ModeDisabled Mode = "disabled"
)

// Default proxy URLs for Kubernetes cluster internal communication
const (
	// DefaultSidecarProxyURL is the default URL for sidecar proxy
	DefaultSidecarProxyURL = "http://envoy-proxy.alt-apps.svc.cluster.local:8085"
	// DefaultEnvoyProxyURL is the default URL for Envoy Dynamic Forward Proxy
	DefaultEnvoyProxyURL = "http://envoy-proxy.alt-apps.svc.cluster.local:8080"
	// DefaultNginxProxyURL is the default URL for nginx-external RSS proxy
	DefaultNginxProxyURL = "http://nginx-external.alt-ingress.svc.cluster.local:8889"
)

// Strategy represents the proxy configuration strategy
type Strategy struct {
	// Mode indicates which proxy type is active
	Mode Mode
	// BaseURL is the base URL of the proxy server
	BaseURL string
	// PathTemplate defines how to construct the proxy path
	// Example: "/proxy/{scheme}://{host}{path}" or "/rss-proxy/{scheme}://{host}{path}"
	PathTemplate string
	// Enabled indicates whether this proxy strategy is active
	Enabled bool
}

// IsEnabled returns true if the strategy is enabled and ready for use.
// Returns false for nil strategies.
func (s *Strategy) IsEnabled() bool {
	if s == nil {
		return false
	}
	return s.Enabled
}
