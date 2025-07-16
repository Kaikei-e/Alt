package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Session represents a user session
type Session struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	KratosSessionID string    `json:"kratos_session_id"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	LastActivityAt  time.Time `json:"last_activity_at"`
	IPAddress       *string   `json:"ip_address,omitempty"`
	UserAgent       *string   `json:"user_agent,omitempty"`
	DeviceInfo      *string   `json:"device_info,omitempty"`
	SessionMetadata *string   `json:"session_metadata,omitempty"`
}

// SessionContext represents session context for requests
type SessionContext struct {
	UserID          uuid.UUID `json:"user_id"`
	TenantID        uuid.UUID `json:"tenant_id"`
	Email           string    `json:"email"`
	Name            string    `json:"name"`
	Role            UserRole  `json:"role"`
	SessionID       string    `json:"session_id"`
	KratosSessionID string    `json:"kratos_session_id"`
	IsActive        bool      `json:"is_active"`
	ExpiresAt       time.Time `json:"expires_at"`
	LastActivityAt  time.Time `json:"last_activity_at"`
}

// CSRFToken represents CSRF token
type CSRFToken struct {
	Token     string    `json:"token"`
	SessionID string    `json:"session_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// NewSession creates a new session with validation
func NewSession(userID uuid.UUID, kratosSessionID string, duration time.Duration) (*Session, error) {
	// Validate inputs
	if kratosSessionID == "" {
		return nil, fmt.Errorf("kratos session ID is required")
	}

	if duration <= 0 {
		return nil, fmt.Errorf("session duration must be positive")
	}

	now := time.Now()
	expiresAt := now.Add(duration)

	session := &Session{
		ID:              uuid.New(),
		UserID:          userID,
		KratosSessionID: kratosSessionID,
		Active:          true,
		CreatedAt:       now,
		ExpiresAt:       expiresAt,
		UpdatedAt:       now,
		LastActivityAt:  now,
	}

	return session, nil
}

// IsExpired returns true if the session has expired
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsValid returns true if the session is active and not expired
func (s *Session) IsValid() bool {
	return s.Active && !s.IsExpired()
}

// UpdateActivity updates the last activity timestamp
func (s *Session) UpdateActivity() {
	now := time.Now()
	s.LastActivityAt = now
	s.UpdatedAt = now
}

// Deactivate marks the session as inactive
func (s *Session) Deactivate() {
	s.Active = false
	s.UpdatedAt = time.Now()
}

// ExtendExpiration extends the session expiration time
func (s *Session) ExtendExpiration(duration time.Duration) error {
	if duration <= 0 {
		return fmt.Errorf("extension duration must be positive")
	}

	s.ExpiresAt = s.ExpiresAt.Add(duration)
	s.UpdatedAt = time.Now()
	return nil
}

// GetRemainingTime returns the remaining time until expiration
func (s *Session) GetRemainingTime() time.Duration {
	if s.IsExpired() {
		return 0
	}
	return s.ExpiresAt.Sub(time.Now())
}

// SetIPAddress sets the IP address for the session
func (s *Session) SetIPAddress(ipAddress string) {
	s.IPAddress = &ipAddress
	s.UpdatedAt = time.Now()
}

// SetUserAgent sets the user agent for the session
func (s *Session) SetUserAgent(userAgent string) {
	s.UserAgent = &userAgent
	s.UpdatedAt = time.Now()
}

// SetDeviceInfo sets the device information for the session
func (s *Session) SetDeviceInfo(deviceInfo string) {
	s.DeviceInfo = &deviceInfo
	s.UpdatedAt = time.Now()
}

// SetSessionMetadata sets the session metadata
func (s *Session) SetSessionMetadata(metadata string) {
	s.SessionMetadata = &metadata
	s.UpdatedAt = time.Now()
}

// NewCSRFToken creates a new CSRF token
func NewCSRFToken(sessionID string, tokenLength int, duration time.Duration) (*CSRFToken, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}

	if tokenLength <= 0 {
		tokenLength = 32 // default token length
	}

	if duration <= 0 {
		return nil, fmt.Errorf("token duration must be positive")
	}

	// Generate cryptographically secure random token
	tokenBytes := make([]byte, tokenLength)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random token: %w", err)
	}

	token := hex.EncodeToString(tokenBytes)
	now := time.Now()

	csrfToken := &CSRFToken{
		Token:     token,
		SessionID: sessionID,
		ExpiresAt: now.Add(duration),
		CreatedAt: now,
	}

	return csrfToken, nil
}

// IsExpired returns true if the CSRF token is expired
func (c *CSRFToken) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// IsValid returns true if the CSRF token is not expired
func (c *CSRFToken) IsValid() bool {
	return !c.IsExpired()
}

// Validate checks if the provided token matches and is valid
func (c *CSRFToken) Validate(token string) error {
	if token == "" {
		return fmt.Errorf("token is required")
	}

	if c.Token != token {
		return fmt.Errorf("token mismatch")
	}

	if c.IsExpired() {
		return fmt.Errorf("token expired")
	}

	return nil
}
