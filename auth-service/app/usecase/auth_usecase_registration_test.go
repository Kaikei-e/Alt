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

func TestAuthUsecase_InitiateRegistration(t *testing.T) {
	tests := []struct {
		name         string
		setupMocks   func(*mock_port.MockAuthRepository, *mock_port.MockAuthGateway)
		wantErr      bool
		wantErrMsg   string
		validateFlow func(*testing.T, *domain.RegistrationFlow)
	}{
		{
			name: "successful registration flow creation",
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				expectedFlow := &domain.RegistrationFlow{
					ID:        "flow-reg-123",
					Type:      "browser",
					ExpiresAt: domain.NewExpirationTime(10), // 10 minutes from now
					IssuedAt:  domain.NewCurrentTime(),
					UI: &domain.AuthFlowUI{
						Action: "https://kratos.local/self-service/registration",
						Method: "POST",
						Nodes: []*domain.AuthFlowNode{
							{
								Type:  "input",
								Group: "default",
								Attributes: map[string]interface{}{
									"name": "traits.email",
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
							{
								Type:  "input",
								Group: "default",
								Attributes: map[string]interface{}{
									"name": "traits.name",
									"type": "text",
								},
							},
						},
					},
				}
				mockGateway.EXPECT().
					CreateRegistrationFlow(gomock.Any()).
					Return(expectedFlow, nil)
			},
			wantErr: false,
			validateFlow: func(t *testing.T, flow *domain.RegistrationFlow) {
				require.NotNil(t, flow)
				assert.Equal(t, "flow-reg-123", flow.ID)
				assert.Equal(t, "browser", flow.Type)
				assert.NotNil(t, flow.UI)
				assert.Equal(t, "https://kratos.local/self-service/registration", flow.UI.Action)
				assert.Equal(t, "POST", flow.UI.Method)
				assert.Len(t, flow.UI.Nodes, 3)
			},
		},
		{
			name: "kratos gateway error",
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				mockGateway.EXPECT().
					CreateRegistrationFlow(gomock.Any()).
					Return(nil, errors.New("kratos connection failed"))
			},
			wantErr:    true,
			wantErrMsg: "kratos connection failed",
		},
		{
			name: "kratos service unavailable",
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				mockGateway.EXPECT().
					CreateRegistrationFlow(gomock.Any()).
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
			flow, err := useCase.InitiateRegistration(context.Background())

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

func TestAuthUsecase_CompleteRegistration(t *testing.T) {
	tests := []struct {
		name           string
		flowID         string
		registrationBody interface{}
		setupMocks     func(*mock_port.MockAuthRepository, *mock_port.MockAuthGateway)
		wantErr        bool
		wantErrMsg     string
		validateResult func(*testing.T, *domain.SessionContext)
	}{
		{
			name:   "successful registration completion",
			flowID: "flow-reg-123",
			registrationBody: map[string]interface{}{
				"traits.email": "newuser@example.com",
				"password":     "secure_password123",
				"traits.name":  "New User",
			},
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				// Mock Kratos session response
				kratosSession := &domain.KratosSession{
					ID:        "kratos-session-new-789",
					Active:    true,
					ExpiresAt: domain.NewExpirationTime(24 * 60), // 24 hours from now
					Identity: &domain.KratosIdentity{
						ID: "650e8400-e29b-41d4-a716-446655440000", // Valid UUID
						Traits: map[string]interface{}{
							"email": "newuser@example.com",
							"name":  "New User",
						},
					},
				}
				
				mockGateway.EXPECT().
					SubmitRegistrationFlow(gomock.Any(), "flow-reg-123", gomock.Any()).
					Return(kratosSession, nil)
				
				// Mock session creation
				mockRepo.EXPECT().
					CreateSession(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			wantErr: false,
			validateResult: func(t *testing.T, sessionCtx *domain.SessionContext) {
				require.NotNil(t, sessionCtx)
				assert.Equal(t, "newuser@example.com", sessionCtx.Email)
				assert.Equal(t, "New User", sessionCtx.Name)
				assert.Equal(t, "kratos-session-new-789", sessionCtx.KratosSessionID)
				assert.True(t, sessionCtx.IsActive)
				assert.Equal(t, domain.UserRoleUser, sessionCtx.Role)
			},
		},
		{
			name:   "registration with invalid email",
			flowID: "flow-reg-123",
			registrationBody: map[string]interface{}{
				"traits.email": "invalid-email",
				"password":     "secure_password123",
				"traits.name":  "New User",
			},
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				mockGateway.EXPECT().
					SubmitRegistrationFlow(gomock.Any(), "flow-reg-123", gomock.Any()).
					Return(nil, errors.New("invalid email format"))
			},
			wantErr:    true,
			wantErrMsg: "invalid email format",
		},
		{
			name:   "registration with weak password",
			flowID: "flow-reg-123",
			registrationBody: map[string]interface{}{
				"traits.email": "user@example.com",
				"password":     "weak",
				"traits.name":  "New User",
			},
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				mockGateway.EXPECT().
					SubmitRegistrationFlow(gomock.Any(), "flow-reg-123", gomock.Any()).
					Return(nil, errors.New("password does not meet security requirements"))
			},
			wantErr:    true,
			wantErrMsg: "password does not meet security requirements",
		},
		{
			name:   "registration with duplicate email",
			flowID: "flow-reg-123",
			registrationBody: map[string]interface{}{
				"traits.email": "existing@example.com",
				"password":     "secure_password123",
				"traits.name":  "New User",
			},
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				mockGateway.EXPECT().
					SubmitRegistrationFlow(gomock.Any(), "flow-reg-123", gomock.Any()).
					Return(nil, errors.New("email already exists"))
			},
			wantErr:    true,
			wantErrMsg: "email already exists",
		},
		{
			name:   "session creation failure during registration",
			flowID: "flow-reg-123",
			registrationBody: map[string]interface{}{
				"traits.email": "user@example.com",
				"password":     "secure_password123",
				"traits.name":  "New User",
			},
			setupMocks: func(mockRepo *mock_port.MockAuthRepository, mockGateway *mock_port.MockAuthGateway) {
				kratosSession := &domain.KratosSession{
					ID:     "kratos-session-new-789",
					Active: true,
					Identity: &domain.KratosIdentity{
						ID: "650e8400-e29b-41d4-a716-446655440000", // Valid UUID
						Traits: map[string]interface{}{
							"email": "user@example.com",
							"name":  "New User",
						},
					},
				}
				
				mockGateway.EXPECT().
					SubmitRegistrationFlow(gomock.Any(), "flow-reg-123", gomock.Any()).
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
			sessionCtx, err := useCase.CompleteRegistration(context.Background(), tt.flowID, tt.registrationBody)

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