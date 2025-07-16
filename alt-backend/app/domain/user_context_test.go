package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestUserContext_IsValid(t *testing.T) {
	validUserID := uuid.New()
	nilUserID := uuid.UUID{}

	tests := []struct {
		name        string
		userContext UserContext
		want        bool
	}{
		{
			name: "valid user context",
			userContext: UserContext{
				UserID:    validUserID,
				Email:     "test@example.com",
				ExpiresAt: time.Now().Add(time.Hour),
			},
			want: true,
		},
		{
			name: "expired user context",
			userContext: UserContext{
				UserID:    validUserID,
				Email:     "test@example.com",
				ExpiresAt: time.Now().Add(-time.Hour),
			},
			want: false,
		},
		{
			name: "empty email",
			userContext: UserContext{
				UserID:    validUserID,
				Email:     "",
				ExpiresAt: time.Now().Add(time.Hour),
			},
			want: false,
		},
		{
			name: "nil user id",
			userContext: UserContext{
				UserID:    nilUserID,
				Email:     "test@example.com",
				ExpiresAt: time.Now().Add(time.Hour),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.userContext.IsValid()
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestUserContext_IsAdmin(t *testing.T) {
	tests := []struct {
		name        string
		userContext UserContext
		want        bool
	}{
		{
			name: "admin user",
			userContext: UserContext{
				Role: UserRoleAdmin,
			},
			want: true,
		},
		{
			name: "regular user",
			userContext: UserContext{
				Role: UserRoleUser,
			},
			want: false,
		},
		{
			name: "empty role",
			userContext: UserContext{
				Role: "",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.userContext.IsAdmin()
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestUserContext_HasPermission(t *testing.T) {
	tests := []struct {
		name        string
		userContext UserContext
		permission  string
		want        bool
	}{
		{
			name: "has permission",
			userContext: UserContext{
				Permissions: []string{"read", "write", "delete"},
			},
			permission: "write",
			want:       true,
		},
		{
			name: "does not have permission",
			userContext: UserContext{
				Permissions: []string{"read", "write"},
			},
			permission: "delete",
			want:       false,
		},
		{
			name: "empty permissions",
			userContext: UserContext{
				Permissions: []string{},
			},
			permission: "read",
			want:       false,
		},
		{
			name: "nil permissions",
			userContext: UserContext{
				Permissions: nil,
			},
			permission: "read",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.userContext.HasPermission(tt.permission)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestUserRole_Constants(t *testing.T) {
	assert.Equal(t, UserRole("user"), UserRoleUser)
	assert.Equal(t, UserRole("admin"), UserRoleAdmin)
}