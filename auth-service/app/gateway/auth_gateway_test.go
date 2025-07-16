package gateway

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"auth-service/app/domain"
	mock_port "auth-service/app/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestAuthGateway_CreateLoginFlow(t *testing.T) {
	tests := []struct {
		name       string
		setupMocks func(*mock_port.MockKratosClient)
		expectErr  bool
	}{
		{
			name: "successful login flow creation",
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				expectedFlow := &domain.LoginFlow{
					ID:        "flow-123",
					Type:      "login",
					ExpiresAt: time.Now().Add(1 * time.Hour),
					IssuedAt:  time.Now(),
					TenantID:  uuid.New(),
				}
				mockClient.EXPECT().
					CreateLoginFlow(gomock.Any(), gomock.Any(), false, "").
					Return(expectedFlow, nil)
			},
			expectErr: false,
		},
		{
			name: "kratos client error",
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				mockClient.EXPECT().
					CreateLoginFlow(gomock.Any(), gomock.Any(), false, "").
					Return(nil, assert.AnError)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mock_port.NewMockKratosClient(ctrl)
			tt.setupMocks(mockClient)

			// Create gateway
			gateway := NewAuthGateway(mockClient, testLogger())

			// Execute
			flow, err := gateway.CreateLoginFlow(context.Background())

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, flow)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, flow)
				assert.Equal(t, "flow-123", flow.ID)
			}
		})
	}
}

func TestAuthGateway_CreateRegistrationFlow(t *testing.T) {
	tests := []struct {
		name       string
		setupMocks func(*mock_port.MockKratosClient)
		expectErr  bool
	}{
		{
			name: "successful registration flow creation",
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				expectedFlow := &domain.RegistrationFlow{
					ID:        "flow-123",
					Type:      "registration",
					ExpiresAt: time.Now().Add(1 * time.Hour),
					IssuedAt:  time.Now(),
					TenantID:  uuid.New(),
				}
				mockClient.EXPECT().
					CreateRegistrationFlow(gomock.Any(), gomock.Any(), "").
					Return(expectedFlow, nil)
			},
			expectErr: false,
		},
		{
			name: "kratos client error",
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				mockClient.EXPECT().
					CreateRegistrationFlow(gomock.Any(), gomock.Any(), "").
					Return(nil, assert.AnError)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mock_port.NewMockKratosClient(ctrl)
			tt.setupMocks(mockClient)

			// Create gateway
			gateway := NewAuthGateway(mockClient, testLogger())

			// Execute
			flow, err := gateway.CreateRegistrationFlow(context.Background())

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, flow)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, flow)
				assert.Equal(t, "flow-123", flow.ID)
			}
		})
	}
}

func TestAuthGateway_SubmitLoginFlow(t *testing.T) {
	tests := []struct {
		name       string
		flowID     string
		body       interface{}
		setupMocks func(*mock_port.MockKratosClient)
		expectErr  bool
	}{
		{
			name:   "successful login flow submission",
			flowID: "flow-123",
			body: map[string]interface{}{
				"email":    "test@example.com",
				"password": "password123",
			},
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				expectedSession := &domain.KratosSession{
					ID:     "session-123",
					Active: true,
					Identity: &domain.KratosIdentity{
						ID: uuid.New().String(),
						Traits: map[string]interface{}{
							"email": "test@example.com",
							"name":  "Test User",
						},
					},
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}
				mockClient.EXPECT().
					SubmitLoginFlow(gomock.Any(), "flow-123", gomock.Any()).
					Return(expectedSession, nil)
			},
			expectErr: false,
		},
		{
			name:   "invalid body type",
			flowID: "flow-123",
			body:   "invalid-body",
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				// No mock expectations as the function should fail early
			},
			expectErr: true,
		},
		{
			name:   "kratos client error",
			flowID: "flow-123",
			body: map[string]interface{}{
				"email":    "test@example.com",
				"password": "password123",
			},
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				mockClient.EXPECT().
					SubmitLoginFlow(gomock.Any(), "flow-123", gomock.Any()).
					Return(nil, assert.AnError)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mock_port.NewMockKratosClient(ctrl)
			tt.setupMocks(mockClient)

			// Create gateway
			gateway := NewAuthGateway(mockClient, testLogger())

			// Execute
			session, err := gateway.SubmitLoginFlow(context.Background(), tt.flowID, tt.body)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, session)
				assert.Equal(t, "session-123", session.ID)
				assert.True(t, session.Active)
			}
		})
	}
}

func TestAuthGateway_GetSession(t *testing.T) {
	tests := []struct {
		name         string
		sessionToken string
		setupMocks   func(*mock_port.MockKratosClient)
		expectErr    bool
	}{
		{
			name:         "successful session retrieval",
			sessionToken: "session-token-123",
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				expectedSession := &domain.KratosSession{
					ID:     "session-123",
					Active: true,
					Identity: &domain.KratosIdentity{
						ID: uuid.New().String(),
						Traits: map[string]interface{}{
							"email": "test@example.com",
							"name":  "Test User",
						},
					},
					ExpiresAt: time.Now().Add(1 * time.Hour),
				}
				mockClient.EXPECT().
					GetSession(gomock.Any(), "session-token-123").
					Return(expectedSession, nil)
			},
			expectErr: false,
		},
		{
			name:         "kratos client error",
			sessionToken: "invalid-token",
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				mockClient.EXPECT().
					GetSession(gomock.Any(), "invalid-token").
					Return(nil, assert.AnError)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mock_port.NewMockKratosClient(ctrl)
			tt.setupMocks(mockClient)

			// Create gateway
			gateway := NewAuthGateway(mockClient, testLogger())

			// Execute
			session, err := gateway.GetSession(context.Background(), tt.sessionToken)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, session)
				assert.Equal(t, "session-123", session.ID)
				assert.True(t, session.Active)
			}
		})
	}
}

func TestAuthGateway_RevokeSession(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		setupMocks func(*mock_port.MockKratosClient)
		expectErr  bool
	}{
		{
			name:      "successful session revocation",
			sessionID: "session-123",
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				mockClient.EXPECT().
					RevokeSession(gomock.Any(), "session-123").
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:      "kratos client error",
			sessionID: "session-123",
			setupMocks: func(mockClient *mock_port.MockKratosClient) {
				mockClient.EXPECT().
					RevokeSession(gomock.Any(), "session-123").
					Return(assert.AnError)
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mock_port.NewMockKratosClient(ctrl)
			tt.setupMocks(mockClient)

			// Create gateway
			gateway := NewAuthGateway(mockClient, testLogger())

			// Execute
			err := gateway.RevokeSession(context.Background(), tt.sessionID)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper function to create a test logger
func testLogger() *slog.Logger {
	return slog.Default()
}