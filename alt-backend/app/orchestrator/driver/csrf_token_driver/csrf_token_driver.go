package csrf_token_driver

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

const maxCSRFTokens = 10000

// InMemoryCSRFTokenDriver implements CSRF token storage using in-memory storage
type InMemoryCSRFTokenDriver struct {
	tokens map[string]time.Time
	mu     sync.RWMutex
}

// NewInMemoryCSRFTokenDriver creates a new in-memory CSRF token driver
func NewInMemoryCSRFTokenDriver() *InMemoryCSRFTokenDriver {
	driver := &InMemoryCSRFTokenDriver{
		tokens: make(map[string]time.Time),
	}

	// Start cleanup goroutine to remove expired tokens
	go driver.cleanupExpiredTokens()

	return driver
}

// StoreToken stores a CSRF token with its expiration time
func (d *InMemoryCSRFTokenDriver) StoreToken(ctx context.Context, token string, expiration time.Time) error {
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.tokens == nil {
		d.tokens = make(map[string]time.Time)
	}

	if _, exists := d.tokens[token]; !exists && len(d.tokens) >= maxCSRFTokens {
		now := time.Now()
		for k, exp := range d.tokens {
			if now.After(exp) {
				delete(d.tokens, k)
			}
		}
		if len(d.tokens) >= maxCSRFTokens {
			var oldestKey string
			var oldestExp time.Time
			first := true
			for k, exp := range d.tokens {
				if first || exp.Before(oldestExp) {
					oldestKey = k
					oldestExp = exp
					first = false
				}
			}
			if !first {
				delete(d.tokens, oldestKey)
			}
		}
	}

	d.tokens[token] = expiration
	return nil
}

// GetToken retrieves a CSRF token's expiration time
func (d *InMemoryCSRFTokenDriver) GetToken(ctx context.Context, token string) (time.Time, error) {
	if token == "" {
		return time.Time{}, fmt.Errorf("token cannot be empty")
	}

	d.mu.RLock()
	expiration, exists := d.tokens[token]
	d.mu.RUnlock()
	if !exists {
		return time.Time{}, fmt.Errorf("token not found")
	}

	return expiration, nil
}

// DeleteToken removes a CSRF token from storage
func (d *InMemoryCSRFTokenDriver) DeleteToken(ctx context.Context, token string) error {
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	d.mu.Lock()
	delete(d.tokens, token)
	d.mu.Unlock()
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

	for range ticker.C {
		now := time.Now()
		d.mu.Lock()
		for token, expiration := range d.tokens {
			if now.After(expiration) {
				delete(d.tokens, token)
			}
		}
		d.mu.Unlock()
	}
}
