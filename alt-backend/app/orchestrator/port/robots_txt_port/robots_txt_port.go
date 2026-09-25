package robots_txt_port

import (
	"alt/domain"
	"context"
	"net/url"
)

//go:generate go run go.uber.org/mock/mockgen -source=robots_txt_port.go -destination=../../mocks/mock_robots_txt_port.go

// RobotsTxtFetcherPort fetches and parses robots.txt for a given domain
type RobotsTxtFetcherPort interface {
	FetchRobotsTxt(ctx context.Context, domainName, scheme string) (*domain.RobotsTxt, error)
}

// RobotsTxtPolicyPort checks path access policy against robots.txt rules
type RobotsTxtPolicyPort interface {
	IsPathAllowed(ctx context.Context, targetURL *url.URL, userAgent string) (bool, error)
}
