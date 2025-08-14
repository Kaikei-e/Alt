package auth_gateway

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"alt/domain"
	"alt/driver/auth"
	"alt/mocks"
)

func TestAuthGateway_ValidateSession(t *testing.T) {
	tests := []struct {
		name         string
		sessionToken string
		setupMock    func(*mocks.MockAuthClient, uuid.UUID)
		wantErr      bool
	}{
		{
			name:         "valid session",
			sessionToken: "valid-session-token",
			setupMock: func(m *mocks.MockAuthClient, userID uuid.UUID) {
				m.EXPECT().
					ValidateSession(gomock.Any(), "valid-session-token", gomock.Any()).
					Return(&auth.SessionValidationResponse{
						Valid:  true,
						UserID: userID.String(),
						Email:  "test@example.com",
						Role:   "user",
					}, nil)
			},
			wantErr: false,
		},
		{
			name:         "invalid session",
			sessionToken: "invalid-session-token",
			setupMock: func(m *mocks.MockAuthClient, _ uuid.UUID) {
				m.EXPECT().
					ValidateSession(gomock.Any(), "invalid-session-token", gomock.Any()).
					Return(&auth.SessionValidationResponse{Valid: false}, nil)
			},
			wantErr: true,
		},
		{
			name:         "auth client error",
			sessionToken: "error-token",
			setupMock: func(m *mocks.MockAuthClient, _ uuid.UUID) {
				m.EXPECT().
					ValidateSession(gomock.Any(), "error-token", gomock.Any()).
					Return(nil, errors.New("client error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockAuthClient(ctrl)
			userID := uuid.New()
			tt.setupMock(mockClient, userID)

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			gateway := NewAuthGateway(mockClient, logger)

			ctx := context.Background()
			result, err := gateway.ValidateSession(ctx, tt.sessionToken)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, userID, result.UserID)
				assert.Equal(t, "test@example.com", result.Email)
				assert.Equal(t, domain.UserRole("user"), result.Role)
			}
		})
	}
}
