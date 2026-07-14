package user_read_state_port

import (
	"context"

	"github.com/google/uuid"
)

type UserReadStatePort interface {
	GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error)
	GetUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
}
