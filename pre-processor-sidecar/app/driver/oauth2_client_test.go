package driver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOAuth2Client(t *testing.T) {
	client := NewOAuth2Client("test_client_id", "test_client_secret", "https://test.example.com")
	
	assert.Equal(t, "test_client_id", client.clientID)
	assert.Equal(t, "test_client_secret", client.clientSecret)
	assert.Equal(t, "https://test.example.com", client.baseURL)
	assert.NotNil(t, client.httpClient)
}

func TestOAuth2Client_RefreshToken(t *testing.T) {
	tests := map[string]struct {
		refreshToken     string
		mockResponse     func() *httptest.Server
		expectError      bool
		expectAccessToken string
		expectExpiresIn   int
	}{
		"valid_refresh_token": {
			refreshToken: "valid_refresh_token",
			mockResponse: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Verify request method and content type
					assert.Equal(t, http.MethodPost, r.Method)
					assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
					
					// Parse form data
					err := r.ParseForm()
					require.NoError(t, err)
					
					// Verify required parameters
					assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
					assert.Equal(t, "valid_refresh_token", r.Form.Get("refresh_token"))
					assert.Equal(t, "test_client_id", r.Form.Get("client_id"))
					assert.Equal(t, "test_client_secret", r.Form.Get("client_secret"))
					
					// Return mock OAuth2 response
					response := map[string]interface{}{
						"access_token": "new_access_token_123",
						"token_type":   "Bearer",
						"expires_in":   3600,
						"refresh_token": "new_refresh_token_456",
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(response)
				}))
			},
			expectError:       false,
			expectAccessToken: "new_access_token_123",
			expectExpiresIn:   3600,
		},
		"invalid_refresh_token": {
			refreshToken: "invalid_refresh_token",
			mockResponse: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
					errorResponse := map[string]interface{}{
						"error":             "invalid_grant",
						"error_description": "Invalid refresh token",
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(errorResponse)
				}))
			},
			expectError: true,
		},
		"network_error": {
			refreshToken: "some_token",
			mockResponse: func() *httptest.Server {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// This should never be called
					t.Error("Server should not be called for network error test")
				}))
				server.Close() // Close immediately to simulate network error
				return server
			},
			expectError: true,
		},
		"malformed_json_response": {
			refreshToken: "some_token",
			mockResponse: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"invalid": json`)) // Malformed JSON
				}))
			},
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mock server
			server := tc.mockResponse()
			if name != "network_error" {
				defer server.Close()
			}

			// Create OAuth2 client with mock server URL
			client := NewOAuth2Client("test_client_id", "test_client_secret", server.URL)
			client.httpClient.Timeout = 1 * time.Second // Short timeout for tests

			// Execute RefreshToken
			ctx := context.Background()
			tokenResponse, err := client.RefreshToken(ctx, tc.refreshToken)

			// Verify results
			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, tokenResponse)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tokenResponse)
				assert.Equal(t, tc.expectAccessToken, tokenResponse.AccessToken)
				assert.Equal(t, "Bearer", tokenResponse.TokenType)
				assert.Equal(t, tc.expectExpiresIn, tokenResponse.ExpiresIn)
				assert.Equal(t, "new_refresh_token_456", tokenResponse.RefreshToken)
				
				// Verify token expiration calculation
				expectedExpiry := time.Now().Add(time.Duration(tc.expectExpiresIn) * time.Second)
				assert.WithinDuration(t, expectedExpiry, tokenResponse.ExpiresAt, 5*time.Second)
			}
		})
	}
}

func TestOAuth2Client_ValidateToken(t *testing.T) {
	tests := map[string]struct {
		accessToken  string
		mockResponse func() *httptest.Server
		expectError  bool
		expectValid  bool
	}{
		"valid_token": {
			accessToken: "valid_access_token",
			mockResponse: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Verify authorization header
					authHeader := r.Header.Get("Authorization")
					assert.Equal(t, "Bearer valid_access_token", authHeader)
					
					// Return user info (token is valid)
					w.WriteHeader(http.StatusOK)
					userInfo := map[string]interface{}{
						"userId":   "123456789",
						"userName": "test_user",
						"userEmail": "test@example.com",
					}
					json.NewEncoder(w).Encode(userInfo)
				}))
			},
			expectError: false,
			expectValid: true,
		},
		"expired_token": {
			accessToken: "expired_access_token",
			mockResponse: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					errorResponse := map[string]interface{}{
						"error":       "invalid_token",
						"description": "Token expired",
					}
					json.NewEncoder(w).Encode(errorResponse)
				}))
			},
			expectError: false,
			expectValid: false,
		},
		"invalid_token": {
			accessToken: "invalid_access_token",
			mockResponse: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					errorResponse := map[string]interface{}{
						"error":       "access_denied",
						"description": "Invalid token",
					}
					json.NewEncoder(w).Encode(errorResponse)
				}))
			},
			expectError: false,
			expectValid: false,
		},
		"network_error": {
			accessToken: "some_token",
			mockResponse: func() *httptest.Server {
				server := httptest.NewServer(nil)
				server.Close() // Simulate network error
				return server
			},
			expectError: true,
			expectValid: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mock server
			server := tc.mockResponse()
			if name != "network_error" {
				defer server.Close()
			}

			// Create OAuth2 client with mock server URL
			client := NewOAuth2Client("test_client_id", "test_client_secret", server.URL)
			client.httpClient.Timeout = 1 * time.Second

			// Execute ValidateToken
			ctx := context.Background()
			isValid, err := client.ValidateToken(ctx, tc.accessToken)

			// Verify results
			if tc.expectError {
				assert.Error(t, err)
				assert.False(t, isValid)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectValid, isValid)
			}
		})
	}
}

func TestOAuth2Client_MakeAuthenticatedRequest(t *testing.T) {
	tests := map[string]struct {
		accessToken  string
		endpoint     string
		mockResponse func() *httptest.Server
		expectError  bool
		expectData   map[string]interface{}
	}{
		"successful_api_call": {
			accessToken: "valid_token",
			endpoint:    "/subscription/list",
			mockResponse: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Verify authorization header
					authHeader := r.Header.Get("Authorization")
					assert.Equal(t, "Bearer valid_token", authHeader)
					assert.Equal(t, "/subscription/list", r.URL.Path)
					
					// Return API response
					w.WriteHeader(http.StatusOK)
					response := map[string]interface{}{
						"subscriptions": []map[string]interface{}{
							{
								"id":    "feed/http://example.com/rss",
								"title": "Example Feed",
								"categories": []map[string]interface{}{
									{"id": "user/123/label/Tech", "label": "Tech"},
								},
							},
						},
					}
					json.NewEncoder(w).Encode(response)
				}))
			},
			expectError: false,
			expectData: map[string]interface{}{
				"subscriptions": []interface{}{
					map[string]interface{}{
						"id":    "feed/http://example.com/rss",
						"title": "Example Feed",
						"categories": []interface{}{
							map[string]interface{}{
								"id":    "user/123/label/Tech",
								"label": "Tech",
							},
						},
					},
				},
			},
		},
		"api_rate_limit_error": {
			accessToken: "valid_token",
			endpoint:    "/stream/contents",
			mockResponse: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("X-Reader-Zone1-Limit", "100")
					w.Header().Set("X-Reader-Zone1-Usage", "100")
					w.WriteHeader(http.StatusTooManyRequests)
					errorResponse := map[string]interface{}{
						"error":       "rate_limit_exceeded",
						"description": "Zone 1 daily limit exceeded",
					}
					json.NewEncoder(w).Encode(errorResponse)
				}))
			},
			expectError: true,
		},
		"unauthorized_error": {
			accessToken: "expired_token",
			endpoint:    "/subscription/list",
			mockResponse: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					errorResponse := map[string]interface{}{
						"error":       "invalid_token",
						"description": "Token expired or invalid",
					}
					json.NewEncoder(w).Encode(errorResponse)
				}))
			},
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mock server
			server := tc.mockResponse()
			defer server.Close()

			// Create OAuth2 client with mock server URL
			client := NewOAuth2Client("test_client_id", "test_client_secret", server.URL)

			// Execute MakeAuthenticatedRequest
			ctx := context.Background()
			data, err := client.MakeAuthenticatedRequest(ctx, tc.accessToken, tc.endpoint, nil)

			// Verify results
			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, data)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectData, data)
			}
		})
	}
}

func TestOAuth2Client_HandleRateLimitHeaders(t *testing.T) {
	tests := map[string]struct {
		headers         map[string]string
		expectedUsage   int
		expectedLimit   int
		expectedRemaining int
	}{
		"with_rate_limit_headers": {
			headers: map[string]string{
				"X-Reader-Zone1-Usage": "45",
				"X-Reader-Zone1-Limit": "100",
			},
			expectedUsage:     45,
			expectedLimit:     100,
			expectedRemaining: 55,
		},
		"without_headers": {
			headers:           map[string]string{},
			expectedUsage:     0,
			expectedLimit:     100, // Default limit
			expectedRemaining: 100,
		},
		"invalid_header_values": {
			headers: map[string]string{
				"X-Reader-Zone1-Usage": "invalid",
				"X-Reader-Zone1-Limit": "not_a_number",
			},
			expectedUsage:     0,
			expectedLimit:     100, // Default limit
			expectedRemaining: 100,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := NewOAuth2Client("test_id", "test_secret", "https://test.example.com")

			usage, limit, remaining := client.handleRateLimitHeaders(tc.headers)

			assert.Equal(t, tc.expectedUsage, usage)
			assert.Equal(t, tc.expectedLimit, limit)
			assert.Equal(t, tc.expectedRemaining, remaining)
		})
	}
}

// Benchmark tests for performance
func BenchmarkOAuth2Client_RefreshToken(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"access_token":  "benchmark_token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "benchmark_refresh_token",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewOAuth2Client("test_client_id", "test_client_secret", server.URL)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.RefreshToken(ctx, "refresh_token")
		if err != nil {
			b.Fatal(err)
		}
	}
}