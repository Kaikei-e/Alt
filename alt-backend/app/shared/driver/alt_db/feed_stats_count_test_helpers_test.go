package alt_db

import (
	"context"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

func authContext() context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "test-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
}
