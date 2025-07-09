package csrf_token_driver

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

// InMemoryCSRFTokenDriver implements CSRF token storage using in-memory storage
type InMemoryCSRFTokenDriver struct {
	tokens sync.Map // map[string]time.Time
	mu     sync.RWMutex
}

// NewInMemoryCSRFTokenDriver creates a new in-memory CSRF token driver
func NewInMemoryCSRFTokenDriver() *InMemoryCSRFTokenDriver {
	driver := &InMemoryCSRFTokenDriver{}

	// Start cleanup goroutine to remove expired tokens
	go driver.cleanupExpiredTokens()

	return driver
}

// StoreToken stores a CSRF token with its expiration time
func (d *InMemoryCSRFTokenDriver) StoreToken(ctx context.Context, token string, expiration time.Time) error {
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	d.tokens.Store(token, expiration)
	return nil
}

// GetToken retrieves a CSRF token's expiration time
func (d *InMemoryCSRFTokenDriver) GetToken(ctx context.Context, token string) (time.Time, error) {
	if token == "" {
		return time.Time{}, fmt.Errorf("token cannot be empty")
	}

	expiration, exists := d.tokens.Load(token)
	if !exists {
		return time.Time{}, fmt.Errorf("token not found")
	}

	return expiration.(time.Time), nil
}

// DeleteToken removes a CSRF token from storage
func (d *InMemoryCSRFTokenDriver) DeleteToken(ctx context.Context, token string) error {
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	d.tokens.Delete(token)
	return nil
}

// GenerateRandomToken generates a cryptographically secure random token
func (d *InMemoryCSRFTokenDriver) GenerateRandomToken() (string, error) {
	// Generate 32 bytes of random data
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64 for safe URL/header transmission
	token := base64.URLEncoding.EncodeToString(bytes)
	return token, nil
}

// cleanupExpiredTokens periodically removes expired tokens from memory
func (d *InMemoryCSRFTokenDriver) cleanupExpiredTokens() {
	ticker := time.NewTicker(10 * time.Minute) // Cleanup every 10 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			d.tokens.Range(func(key, value interface{}) bool {
				token := key.(string)
				expiration := value.(time.Time)

				if now.After(expiration) {
					d.tokens.Delete(token)
				}

				return true // Continue iteration
			})
		}
	}
}
