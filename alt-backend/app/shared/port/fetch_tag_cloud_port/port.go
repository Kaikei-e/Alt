package fetch_tag_cloud_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

//go:generate mockgen -source=port.go -destination=../../mocks/mock_fetch_tag_cloud_port.go -package=mocks

// FetchTagCloudPort defines the interface for fetching tag cloud data.
type FetchTagCloudPort interface {
	FetchTagCloud(ctx context.Context, limit int) ([]*domain.TagCloudItem, error)
	FetchTagCloudForUser(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.TagCloudItem, error)
	FetchTagCooccurrences(ctx context.Context, tagNames []string) ([]*domain.TagCooccurrence, error)
}
