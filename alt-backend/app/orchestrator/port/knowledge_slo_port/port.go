package knowledge_slo_port

import (
	"context"
	"time"
)

// GetProjectionLagPort returns the current projection lag.
type GetProjectionLagPort interface {
	GetProjectionLag(ctx context.Context) (time.Duration, error)
}

// GetProjectionAgePort returns the age of the latest projection update.
type GetProjectionAgePort interface {
	GetProjectionAge(ctx context.Context) (time.Duration, error)
}
