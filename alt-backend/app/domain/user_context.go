package domain

import (
	"time"

	"github.com/google/uuid"
)

// UserRole represents the role of a user
type UserRole string

const (
	UserRoleUser  UserRole = "user"
	UserRoleAdmin UserRole = "admin"
)

// UserContext represents the authenticated user context for requests
type UserContext struct {
	UserID      uuid.UUID `json:"user_id"`
	Email       string    `json:"email"`
	Role        UserRole  `json:"role"`
	TenantID    uuid.UUID `json:"tenant_id"`
	SessionID   string    `json:"session_id"`
	LoginAt     time.Time `json:"login_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Permissions []string  `json:"permissions,omitempty"`
}

// IsValid checks if the user context is valid and not expired
func (uc *UserContext) IsValid() bool {
	return uc.UserID.String() != "00000000-0000-0000-0000-000000000000" &&
		uc.Email != "" &&
		uc.ExpiresAt.After(time.Now())
}

// IsAdmin checks if the user has admin role
func (uc *UserContext) IsAdmin() bool {
	return uc.Role == UserRoleAdmin
}

// HasPermission checks if the user has a specific permission
func (uc *UserContext) HasPermission(permission string) bool {
	for _, p := range uc.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}