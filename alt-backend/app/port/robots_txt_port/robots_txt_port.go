package robots_txt_port

import (
	"alt/domain"
	"context"
)

//go:generate go run go.uber.org/mock/mockgen -source=robots_txt_port.go -destination=../../mocks/mock_robots_txt_port.go

// RobotsTxtPort defines the interface for robots.txt operations
type RobotsTxtPort interface {
	// FetchRobotsTxt fetches and parses robots.txt for a given domain
	FetchRobotsTxt(ctx context.Context, domainName, scheme string) (*domain.RobotsTxt, error)
}
