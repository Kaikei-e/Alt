package usecase

import (
	"context"
	"errors"
	"testing"

	"auth-service/app/domain"
	mock_port "auth-service/app/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAuthUsecase_InitiateLogin(t *testing.T) {
	tests := []struct {
		name         string
		setupMocks   func(*mock_port.MockAuthRepository, *mock_port.MockAuthGateway)
		wantErr      bool
		wantErrMsg   string
		validateFlow func(*testing.T, *domain.LoginFlow)
	}{
		{
			name: "successful login flow creation",
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				expectedFlow := &domain.LoginFlow{
					ID:        "flow-123",
					Type:      "browser",
					ExpiresAt: domain.NewExpirationTime(10), // 10 minutes from now
					IssuedAt:  domain.NewCurrentTime(),
					UI: &domain.AuthFlowUI{
						Action: "https://kratos.local/self-service/login",
						Method: "POST",
						Nodes: []*domain.AuthFlowNode{
							{
								Type:  "input",
								Group: "default",
								Attributes: map[string]interface{}{
									"name": "identifier",
									"type": "email",
								},
							},
							{
								Type:  "input",
								Group: "password",
								Attributes: map[string]interface{}{
									"name": "password",
									"type": "password",
								},
							},
						},
					},
				}
				mockGateway.EXPECT().
					CreateLoginFlow(gomock.Any()).
					Return(expectedFlow, nil)
			},
			wantErr: false,
			validateFlow: func(t *testing.T, flow *domain.LoginFlow) {
				require.NotNil(t, flow)
				assert.Equal(t, "flow-123", flow.ID)
				assert.Equal(t, "browser", flow.Type)
				assert.NotNil(t, flow.UI)
				assert.Equal(t, "https://kratos.local/self-service/login", flow.UI.Action)
				assert.Equal(t, "POST", flow.UI.Method)
				assert.Len(t, flow.UI.Nodes, 2)
			},
		},
		{
			name: "kratos gateway error",
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				mockGateway.EXPECT().
					CreateLoginFlow(gomock.Any()).
					Return(nil, errors.New("kratos connection failed"))
			},
			wantErr:    true,
			wantErrMsg: "kratos connection failed",
		},
		{
			name: "kratos service unavailable", 
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				mockGateway.EXPECT().
					CreateLoginFlow(gomock.Any()).
					Return(nil, errors.New("service unavailable"))
			},
			wantErr:    true,
			wantErrMsg: "service unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mock_port.NewMockAuthRepository(ctrl)
			mockGateway := mock_port.NewMockAuthGateway(ctrl)
			tt.setupMocks(mockRepo, mockGateway)

			// Create use case
			useCase := NewAuthUseCase(mockRepo, mockGateway)

			// Execute
			flow, err := useCase.InitiateLogin(context.Background())

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				assert.Nil(t, flow)
			} else {
				assert.NoError(t, err)
				if tt.validateFlow != nil {
					tt.validateFlow(t, flow)
				}
			}
		})
	}
}

func TestAuthUsecase_CompleteLogin(t *testing.T) {
	tests := []struct {
		name           string
		flowID         string
		loginBody      interface{}
		setupMocks     func(*mock_port.MockAuthRepository, *mock_port.MockAuthGateway)
		wantErr        bool
		wantErrMsg     string
		validateResult func(*testing.T, *domain.SessionContext)
	}{
		{
			name:   "successful login completion",
			flowID: "flow-123",
			loginBody: map[string]interface{}{
				"identifier": "test@example.com",
				"password":   "secure_password",
			},
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				// Mock Kratos session response
				kratosSession := &domain.KratosSession{
					ID:        "kratos-session-456",
					Active:    true,
					ExpiresAt: domain.NewExpirationTime(24 * 60), // 24 hours from now
					Identity: &domain.KratosIdentity{
						ID: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID
						Traits: map[string]interface{}{
							"email": "test@example.com",
							"name":  "Test User",
						},
					},
				}
				
				mockGateway.EXPECT().
					SubmitLoginFlow(gomock.Any(), "flow-123", gomock.Any()).
					Return(kratosSession, nil)
				
				// Mock session creation
				mockRepo.EXPECT().
					CreateSession(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			wantErr: false,
			validateResult: func(t *testing.T, sessionCtx *domain.SessionContext) {
				require.NotNil(t, sessionCtx)
				assert.Equal(t, "test@example.com", sessionCtx.Email)
				assert.Equal(t, "Test User", sessionCtx.Name)
				assert.Equal(t, "kratos-session-456", sessionCtx.KratosSessionID)
				assert.True(t, sessionCtx.IsActive)
				assert.Equal(t, domain.UserRoleUser, sessionCtx.Role)
			},
		},
		{
			name:   "invalid login credentials",
			flowID: "flow-123",
			loginBody: map[string]interface{}{
				"identifier": "test@example.com",
				"password":   "wrong_password",
			},
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				mockGateway.EXPECT().
					SubmitLoginFlow(gomock.Any(), "flow-123", gomock.Any()).
					Return(nil, errors.New("invalid credentials"))
			},
			wantErr:    true,
			wantErrMsg: "invalid credentials",
		},
		{
			name:   "invalid user uuid from kratos",
			flowID: "flow-123",
			loginBody: map[string]interface{}{
				"identifier": "test@example.com",
				"password":   "secure_password",
			},
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				kratosSession := &domain.KratosSession{
					ID:     "kratos-session-456",
					Active: true,
					Identity: &domain.KratosIdentity{
						ID: "invalid-uuid", // Invalid UUID format
						Traits: map[string]interface{}{
							"email": "test@example.com",
						},
					},
				}
				
				mockGateway.EXPECT().
					SubmitLoginFlow(gomock.Any(), "flow-123", gomock.Any()).
					Return(kratosSession, nil)
			},
			wantErr:    true,
			wantErrMsg: "invalid UUID",
		},
		{
			name:   "session creation failure",
			flowID: "flow-123",
			loginBody: map[string]interface{}{
				"identifier": "test@example.com",
				"password":   "secure_password",
			},
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				kratosSession := &domain.KratosSession{
					ID:     "kratos-session-456",
					Active: true,
					Identity: &domain.KratosIdentity{
						ID: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID
						Traits: map[string]interface{}{
							"email": "test@example.com",
						},
					},
				}
				
				mockGateway.EXPECT().
					SubmitLoginFlow(gomock.Any(), "flow-123", gomock.Any()).
					Return(kratosSession, nil)
				
				mockRepo.EXPECT().
					CreateSession(gomock.Any(), gomock.Any()).
					Return(errors.New("database connection failed"))
			},
			wantErr:    true,
			wantErrMsg: "database connection failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mock_port.NewMockAuthRepository(ctrl)
			mockGateway := mock_port.NewMockAuthGateway(ctrl)
			tt.setupMocks(mockRepo, mockGateway)

			// Create use case
			useCase := NewAuthUseCase(mockRepo, mockGateway)

			// Execute
			sessionCtx, err := useCase.CompleteLogin(context.Background(), tt.flowID, tt.loginBody)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				assert.Nil(t, sessionCtx)
			} else {
				assert.NoError(t, err)
				if tt.validateResult != nil {
					tt.validateResult(t, sessionCtx)
				}
			}
		})
	}
}