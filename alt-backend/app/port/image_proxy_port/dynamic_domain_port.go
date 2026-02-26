package image_proxy_port

import "context"

// DynamicDomainPort defines the interface for dynamic domain allowlisting.
type DynamicDomainPort interface {
	IsAllowedImageDomain(ctx context.Context, hostname string) (bool, error)
}
