package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Config struct {
	AuthServiceURL string
	ServiceName    string
	ServiceSecret  string
	TokenTTL       time.Duration
}

type Client struct {
	config     Config
	httpClient *http.Client
	tokenCache map[string]*CachedToken
}

type CachedToken struct {
	Token     string
	ExpiresAt time.Time
}

type UserContext struct {
	UserID    uuid.UUID `json:"user_id"`
	TenantID  uuid.UUID `json:"tenant_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	SessionID string    `json:"session_id"`
}

type ServiceToken struct {
	ServiceName string    `json:"service_name"`
	IssuedAt    time.Time `json:"iat"`
	ExpiresAt   time.Time `json:"exp"`
	Permissions []string  `json:"permissions"`
	jwt.RegisteredClaims
}

// JWT v5 Claims interface implementation
func (st ServiceToken) GetExpirationTime() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(st.ExpiresAt), nil
}

func (st ServiceToken) GetIssuedAt() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(st.IssuedAt), nil
}

func (st ServiceToken) GetNotBefore() (*jwt.NumericDate, error) {
	return nil, nil
}

func (st ServiceToken) GetIssuer() (string, error) {
	return st.ServiceName, nil
}

func (st ServiceToken) GetSubject() (string, error) {
	return st.ServiceName, nil
}

func (st ServiceToken) GetAudience() (jwt.ClaimStrings, error) {
	return jwt.ClaimStrings{}, nil
}

func NewClient(config Config) *Client {
	return &Client{
		config:     config,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		tokenCache: make(map[string]*CachedToken),
	}
}

// サービストークン生成
func (c *Client) GenerateServiceToken(ctx context.Context) (string, error) {
	claims := ServiceToken{
		ServiceName: c.config.ServiceName,
		IssuedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(c.config.TokenTTL),
		Permissions: []string{"read", "write"}, // サービスに応じて設定
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(c.config.ServiceSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ユーザートークン検証
func (c *Client) ValidateUserToken(ctx context.Context, tokenString string) (*UserContext, error) {
	// キャッシュチェック
	if cached, exists := c.tokenCache[tokenString]; exists && cached.ExpiresAt.After(time.Now()) {
		return c.parseUserToken(cached.Token)
	}

	// auth-serviceで検証
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.AuthServiceURL+"/v1/internal/validate", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenString)
	req.Header.Set("X-Service-Name", c.config.ServiceName)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid token: status %d", resp.StatusCode)
	}

	var userContext UserContext
	if err := json.NewDecoder(resp.Body).Decode(&userContext); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// キャッシュに保存
	c.tokenCache[tokenString] = &CachedToken{
		Token:     tokenString,
		ExpiresAt: time.Now().Add(5 * time.Minute), // 短期キャッシュ
	}

	return &userContext, nil
}

func (c *Client) parseUserToken(tokenString string) (*UserContext, error) {
	// JWT解析ロジック（簡易実装）
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(c.config.ServiceSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID, _ := uuid.Parse(claims["user_id"].(string))
		tenantID, _ := uuid.Parse(claims["tenant_id"].(string))

		return &UserContext{
			UserID:    userID,
			TenantID:  tenantID,
			Email:     claims["email"].(string),
			Role:      claims["role"].(string),
			SessionID: claims["session_id"].(string),
		}, nil
	}

	return nil, fmt.Errorf("invalid token claims")
}
