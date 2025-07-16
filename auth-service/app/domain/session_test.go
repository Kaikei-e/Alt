package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewSession(t *testing.T) {
	tests := []struct {
		name           string
		userID         uuid.UUID
		kratosSessionID string
		duration       time.Duration
		expectErr      bool
	}{
		{
			name:           "valid session creation",
			userID:         uuid.New(),
			kratosSessionID: "kratos-session-123",
			duration:       24 * time.Hour,
			expectErr:      false,
		},
		{
			name:           "empty kratos session ID",
			userID:         uuid.New(),
			kratosSessionID: "",
			duration:       24 * time.Hour,
			expectErr:      true,
		},
		{
			name:           "zero duration",
			userID:         uuid.New(),
			kratosSessionID: "kratos-session-123",
			duration:       0,
			expectErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := NewSession(tt.userID, tt.kratosSessionID, tt.duration)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, session)
				assert.Equal(t, tt.userID, session.UserID)
				assert.Equal(t, tt.kratosSessionID, session.KratosSessionID)
				assert.True(t, session.Active)
				assert.False(t, session.IsExpired())
			}
		})
	}
}

func TestSession_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		expiresAt time.Time
		expected bool
	}{
		{
			name:     "not expired",
			expiresAt: time.Now().Add(1 * time.Hour),
			expected: false,
		},
		{
			name:     "expired",
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected: true,
		},
		{
			name:     "expires now",
			expiresAt: time.Now(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &Session{
				ID:               uuid.New(),
				UserID:           uuid.New(),
				KratosSessionID:  "test-session",
				Active:           true,
				CreatedAt:        time.Now(),
				ExpiresAt:        tt.expiresAt,
				UpdatedAt:        time.Now(),
				LastActivityAt:   time.Now(),
			}

			assert.Equal(t, tt.expected, session.IsExpired())
		})
	}
}

func TestSession_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		active   bool
		expiresAt time.Time
		expected bool
	}{
		{
			name:     "valid session",
			active:   true,
			expiresAt: time.Now().Add(1 * time.Hour),
			expected: true,
		},
		{
			name:     "inactive session",
			active:   false,
			expiresAt: time.Now().Add(1 * time.Hour),
			expected: false,
		},
		{
			name:     "expired session",
			active:   true,
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected: false,
		},
		{
			name:     "inactive and expired",
			active:   false,
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &Session{
				ID:               uuid.New(),
				UserID:           uuid.New(),
				KratosSessionID:  "test-session",
				Active:           tt.active,
				CreatedAt:        time.Now(),
				ExpiresAt:        tt.expiresAt,
				UpdatedAt:        time.Now(),
				LastActivityAt:   time.Now(),
			}

			assert.Equal(t, tt.expected, session.IsValid())
		})
	}
}

func TestSession_UpdateActivity(t *testing.T) {
	session := &Session{
		ID:               uuid.New(),
		UserID:           uuid.New(),
		KratosSessionID:  "test-session",
		Active:           true,
		CreatedAt:        time.Now().Add(-1 * time.Hour),
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		UpdatedAt:        time.Now().Add(-30 * time.Minute),
		LastActivityAt:   time.Now().Add(-30 * time.Minute),
	}

	oldUpdatedAt := session.UpdatedAt
	oldLastActivityAt := session.LastActivityAt

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	session.UpdateActivity()

	assert.True(t, session.LastActivityAt.After(oldLastActivityAt))
	assert.True(t, session.UpdatedAt.After(oldUpdatedAt))
}

func TestSession_Deactivate(t *testing.T) {
	session := &Session{
		ID:               uuid.New(),
		UserID:           uuid.New(),
		KratosSessionID:  "test-session",
		Active:           true,
		CreatedAt:        time.Now(),
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		UpdatedAt:        time.Now(),
		LastActivityAt:   time.Now(),
	}

	oldUpdatedAt := session.UpdatedAt

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	session.Deactivate()

	assert.False(t, session.Active)
	assert.True(t, session.UpdatedAt.After(oldUpdatedAt))
}