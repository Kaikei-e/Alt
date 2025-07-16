package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"auth-service/app/domain"
)

func TestUser_NewUser(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		tenantID uuid.UUID
		kratosID uuid.UUID
		wantErr  bool
	}{
		{
			name:     "valid user creation",
			email:    "test@example.com",
			tenantID: uuid.New(),
			kratosID: uuid.New(),
			wantErr:  false,
		},
		{
			name:     "invalid email",
			email:    "invalid-email",
			tenantID: uuid.New(),
			kratosID: uuid.New(),
			wantErr:  true,
		},
		{
			name:     "empty email",
			email:    "",
			tenantID: uuid.New(),
			kratosID: uuid.New(),
			wantErr:  true,
		},
		{
			name:     "zero tenant ID",
			email:    "test@example.com",
			tenantID: uuid.UUID{},
			kratosID: uuid.New(),
			wantErr:  true,
		},
		{
			name:     "zero kratos ID",
			email:    "test@example.com",
			tenantID: uuid.New(),
			kratosID: uuid.UUID{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := domain.NewUser(tt.email, tt.tenantID, tt.kratosID)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.email, user.Email)
				assert.Equal(t, tt.tenantID, user.TenantID)
				assert.Equal(t, tt.kratosID, user.KratosID)
				assert.Equal(t, domain.UserRoleUser, user.Role)
				assert.Equal(t, domain.UserStatusActive, user.Status)
				assert.False(t, user.CreatedAt.IsZero())
				assert.False(t, user.UpdatedAt.IsZero())
				assert.Nil(t, user.LastLoginAt)
			}
		})
	}
}

func TestUser_UpdateProfile(t *testing.T) {
	user, err := domain.NewUser("test@example.com", uuid.New(), uuid.New())
	require.NoError(t, err)

	originalUpdatedAt := user.UpdatedAt

	// Update profile
	err = user.UpdateProfile("John Doe", domain.UserPreferences{
		Theme:    "dark",
		Language: "en",
	})

	require.NoError(t, err)
	assert.Equal(t, "John Doe", user.Name)
	assert.Equal(t, "dark", user.Preferences.Theme)
	assert.Equal(t, "en", user.Preferences.Language)
	assert.True(t, user.UpdatedAt.After(originalUpdatedAt))
}

func TestUser_RecordLogin(t *testing.T) {
	user, err := domain.NewUser("test@example.com", uuid.New(), uuid.New())
	require.NoError(t, err)

	loginTime := time.Now()
	user.RecordLogin(loginTime)

	assert.NotNil(t, user.LastLoginAt)
	assert.True(t, user.LastLoginAt.Equal(loginTime))
}

func TestUser_ChangeRole(t *testing.T) {
	user, err := domain.NewUser("test@example.com", uuid.New(), uuid.New())
	require.NoError(t, err)

	tests := []struct {
		name    string
		role    domain.UserRole
		wantErr bool
	}{
		{
			name:    "valid role change to admin",
			role:    domain.UserRoleAdmin,
			wantErr: false,
		},
		{
			name:    "valid role change to readonly",
			role:    domain.UserRoleReadonly,
			wantErr: false,
		},
		{
			name:    "invalid role",
			role:    domain.UserRole("invalid"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := user.ChangeRole(tt.role)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.role, user.Role)
			}
		})
	}
}

func TestUser_IsActive(t *testing.T) {
	user, err := domain.NewUser("test@example.com", uuid.New(), uuid.New())
	require.NoError(t, err)

	// Should be active by default
	assert.True(t, user.IsActive())

	// Change status to inactive
	user.Status = domain.UserStatusInactive
	assert.False(t, user.IsActive())

	// Change status to suspended
	user.Status = domain.UserStatusSuspended
	assert.False(t, user.IsActive())
}
