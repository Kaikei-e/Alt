package usecase

import (
	"context"
	"testing"
	"time"

	"auth-service/app/domain"
	mock_port "auth-service/app/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestAuthUseCase_CreateSession(t *testing.T) {
	tests := []struct {
		name            string
		userID          uuid.UUID
		kratosSessionID string
		duration        time.Duration
		setupMocks      func(*mock_port.MockAuthRepository, *mock_port.MockAuthGateway)
		expectErr       bool
	}{
		{
			name:            "successful session creation",
			userID:          uuid.New(),
			kratosSessionID: "kratos-session-123",
			duration:        24 * time.Hour,
			setupMocks: func(repo *mock_port.MockAuthRepository, gateway *mock_port.MockAuthGateway) {
				repo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(nil)
			},
			expectErr: false,
		},
		{
			name:            "repository error",
			userID:          uuid.New(),
			kratosSessionID: "kratos-session-123",
			duration:        24 * time.Hour,
			setupMocks: func(repo *mock_port.MockAuthRepository, gateway *mock_port.MockAuthGateway) {
				repo.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(assert.AnError)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mock_port.NewMockAuthRepository(ctrl)
			mockGateway := mock_port.NewMockAuthGateway(ctrl)
			tt.setupMocks(mockRepo, mockGateway)

			// Create use case
			useCase := NewAuthUseCase(mockRepo, mockGateway)

			// Execute
			session, err := useCase.CreateSession(context.Background(), tt.userID, tt.kratosSessionID, tt.duration)

			// Assert
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

func TestAuthUseCase_ValidateSession(t *testing.T) {
	tests := []struct {
		name         string
		sessionToken string
		setupMocks   func(*mock_port.MockAuthRepository, *mock_port.MockAuthGateway)
		expectErr    bool
		validateFunc func(*testing.T, *domain.SessionContext)
	}{
		{
			name:         "valid session",
			sessionToken: "session-token-123",
			setupMocks: func(repo *mock_port.MockAuthRepository, gateway *mock_port.MockAuthGateway) {
				userID := uuid.New()
				kratosSession := &domain.KratosSession{
					ID: "kratos-session-123",
					Identity: &domain.KratosIdentity{
						ID: userID.String(),
						Traits: map[string]interface{}{
							"email": "test@example.com",
							"name":  "Test User",
						},
					},
				}
				session := &domain.Session{
					ID:              uuid.New(),
					UserID:          userID,
					KratosSessionID: "kratos-session-123",
					Active:          true,
					CreatedAt:       time.Now(),
					ExpiresAt:       time.Now().Add(1 * time.Hour),
					UpdatedAt:       time.Now(),
					LastActivityAt:  time.Now(),
				}
				
				gateway.EXPECT().GetSession(gomock.Any(), "session-token-123").Return(kratosSession, nil)
				repo.EXPECT().GetSessionByKratosID(gomock.Any(), "kratos-session-123").Return(session, nil)
			},
			expectErr: false,
			validateFunc: func(t *testing.T, ctx *domain.SessionContext) {
				assert.NotNil(t, ctx)
				assert.Equal(t, "test@example.com", ctx.Email)
				assert.Equal(t, "Test User", ctx.Name)
				assert.True(t, ctx.IsActive)
				assert.Equal(t, domain.UserRoleUser, ctx.Role)
			},
		},
		{
			name:         "kratos session not found",
			sessionToken: "invalid-token",
			setupMocks: func(repo *mock_port.MockAuthRepository, gateway *mock_port.MockAuthGateway) {
				gateway.EXPECT().GetSession(gomock.Any(), "invalid-token").Return(nil, assert.AnError)
			},
			expectErr: true,
			validateFunc: func(t *testing.T, ctx *domain.SessionContext) {
				assert.Nil(t, ctx)
			},
		},
		{
			name:         "local session not found",
			sessionToken: "session-token-123",
			setupMocks: func(repo *mock_port.MockAuthRepository, gateway *mock_port.MockAuthGateway) {
				kratosSession := &domain.KratosSession{
					ID: "kratos-session-123",
					Identity: &domain.KratosIdentity{
						ID: uuid.New().String(),
						Traits: map[string]interface{}{
							"email": "test@example.com",
						},
					},
				}
				
				gateway.EXPECT().GetSession(gomock.Any(), "session-token-123").Return(kratosSession, nil)
				repo.EXPECT().GetSessionByKratosID(gomock.Any(), "kratos-session-123").Return(nil, assert.AnError)
			},
			expectErr: true,
			validateFunc: func(t *testing.T, ctx *domain.SessionContext) {
				assert.Nil(t, ctx)
			},
		},
		{
			name:         "expired session",
			sessionToken: "session-token-123",
			setupMocks: func(repo *mock_port.MockAuthRepository, gateway *mock_port.MockAuthGateway) {
				userID := uuid.New()
				kratosSession := &domain.KratosSession{
					ID: "kratos-session-123",
					Identity: &domain.KratosIdentity{
						ID: userID.String(),
						Traits: map[string]interface{}{
							"email": "test@example.com",
						},
					},
				}
				session := &domain.Session{
					ID:              uuid.New(),
					UserID:          userID,
					KratosSessionID: "kratos-session-123",
					Active:          true,
					CreatedAt:       time.Now().Add(-2 * time.Hour),
					ExpiresAt:       time.Now().Add(-1 * time.Hour), // expired
					UpdatedAt:       time.Now(),
					LastActivityAt:  time.Now(),
				}
				
				gateway.EXPECT().GetSession(gomock.Any(), "session-token-123").Return(kratosSession, nil)
				repo.EXPECT().GetSessionByKratosID(gomock.Any(), "kratos-session-123").Return(session, nil)
			},
			expectErr: true,
			validateFunc: func(t *testing.T, ctx *domain.SessionContext) {
				assert.Nil(t, ctx)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mock_port.NewMockAuthRepository(ctrl)
			mockGateway := mock_port.NewMockAuthGateway(ctrl)
			tt.setupMocks(mockRepo, mockGateway)

			// Create use case
			useCase := NewAuthUseCase(mockRepo, mockGateway)

			// Execute
			sessionContext, err := useCase.ValidateSession(context.Background(), tt.sessionToken)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			tt.validateFunc(t, sessionContext)
		})
	}
}

func TestAuthUseCase_DeactivateSession(t *testing.T) {
	tests := []struct {
		name            string
		kratosSessionID string
		setupMocks      func(*mock_port.MockAuthRepository, *mock_port.MockAuthGateway)
		expectErr       bool
	}{
		{
			name:            "successful deactivation",
			kratosSessionID: "kratos-session-123",
			setupMocks: func(repo *mock_port.MockAuthRepository, gateway *mock_port.MockAuthGateway) {
				session := &domain.Session{
					ID:              uuid.New(),
					UserID:          uuid.New(),
					KratosSessionID: "kratos-session-123",
					Active:          true,
					CreatedAt:       time.Now(),
					ExpiresAt:       time.Now().Add(1 * time.Hour),
					UpdatedAt:       time.Now(),
					LastActivityAt:  time.Now(),
				}
				repo.EXPECT().GetSessionByKratosID(gomock.Any(), "kratos-session-123").Return(session, nil)
				repo.EXPECT().UpdateSessionStatus(gomock.Any(), session.ID.String(), false).Return(nil)
			},
			expectErr: false,
		},
		{
			name:            "session not found",
			kratosSessionID: "kratos-session-123",
			setupMocks: func(repo *mock_port.MockAuthRepository, gateway *mock_port.MockAuthGateway) {
				repo.EXPECT().GetSessionByKratosID(gomock.Any(), "kratos-session-123").Return(nil, assert.AnError)
			},
			expectErr: true,
		},
		{
			name:            "update error",
			kratosSessionID: "kratos-session-123",
			setupMocks: func(repo *mock_port.MockAuthRepository, gateway *mock_port.MockAuthGateway) {
				session := &domain.Session{
					ID:              uuid.New(),
					UserID:          uuid.New(),
					KratosSessionID: "kratos-session-123",
					Active:          true,
					CreatedAt:       time.Now(),
					ExpiresAt:       time.Now().Add(1 * time.Hour),
					UpdatedAt:       time.Now(),
					LastActivityAt:  time.Now(),
				}
				repo.EXPECT().GetSessionByKratosID(gomock.Any(), "kratos-session-123").Return(session, nil)
				repo.EXPECT().UpdateSessionStatus(gomock.Any(), session.ID.String(), false).Return(assert.AnError)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mock_port.NewMockAuthRepository(ctrl)
			mockGateway := mock_port.NewMockAuthGateway(ctrl)
			tt.setupMocks(mockRepo, mockGateway)

			// Create use case
			useCase := NewAuthUseCase(mockRepo, mockGateway)

			// Execute
			err := useCase.DeactivateSession(context.Background(), tt.kratosSessionID)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
