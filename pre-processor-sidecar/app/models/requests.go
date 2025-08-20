// ABOUTME: Request types for API handlers
package models

// TokenUpdateRequest はトークン更新リクエスト
type TokenUpdateRequest struct {
	RefreshToken  string `json:"refresh_token"`
	ClientID      string `json:"client_id,omitempty"`
	ClientSecret  string `json:"client_secret,omitempty"`
}

// TokenStatusResponse はトークン状態レスポンス
type TokenStatusResponse struct {
	IsValid    bool      `json:"is_valid"`
	ExpiresAt  string    `json:"expires_at,omitempty"`
	LastUpdate string    `json:"last_update,omitempty"`
	Status     string    `json:"status"`
}

// AdminAPIResponse は管理API標準レスポンス
type AdminAPIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}