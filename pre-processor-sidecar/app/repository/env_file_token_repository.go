package repository

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"pre-processor-sidecar/models"
)

// EnvFileTokenRepository implements OAuth2TokenRepository using .env file storage
type EnvFileTokenRepository struct {
	filePath string
	logger   *slog.Logger
	mu       sync.RWMutex
}

// NewEnvFileTokenRepository creates a new .env file-based token repository
func NewEnvFileTokenRepository(filePath string, logger *slog.Logger) *EnvFileTokenRepository {
	if logger == nil {
		logger = slog.Default()
	}

	repo := &EnvFileTokenRepository{
		filePath: filePath,
		logger:   logger,
	}

	// Ensure the directory exists
	if err := os.MkdirAll("/tmp", 0755); err != nil {
		logger.Warn("Failed to create directory for .env file", "error", err)
	}

	return repo
}

// GetCurrentToken retrieves the current OAuth2 token from .env file
func (r *EnvFileTokenRepository) GetCurrentToken(ctx context.Context) (*models.OAuth2Token, error) {
	return r.LoadToken()
}

// SaveToken saves the OAuth2 token to .env file
func (r *EnvFileTokenRepository) SaveToken(ctx context.Context, token *models.OAuth2Token) error {
	return r.saveToken(token)
}

// UpdateToken updates the OAuth2 token in .env file
func (r *EnvFileTokenRepository) UpdateToken(ctx context.Context, token *models.OAuth2Token) error {
	return r.saveToken(token)
}

// saveToken is the internal implementation for saving tokens
func (r *EnvFileTokenRepository) saveToken(token *models.OAuth2Token) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("Saving OAuth2 token to .env file", "file_path", r.filePath)

	// Read existing .env content (excluding OAuth2 tokens)
	existingContent := r.readNonTokenLines()

	// Create new .env content with updated tokens
	file, err := os.OpenFile(r.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open .env file: %w", err)
	}
	defer file.Close()

	// Write existing non-token content
	for _, line := range existingContent {
		if _, err := file.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("failed to write existing content: %w", err)
		}
	}

	// Write OAuth2 token variables
	tokenContent := []string{
		fmt.Sprintf("OAUTH2_ACCESS_TOKEN=%s", token.AccessToken),
		fmt.Sprintf("OAUTH2_REFRESH_TOKEN=%s", token.RefreshToken),
		fmt.Sprintf("OAUTH2_TOKEN_TYPE=%s", token.TokenType),
		fmt.Sprintf("OAUTH2_EXPIRES_AT=%s", token.ExpiresAt.Format(time.RFC3339)),
		fmt.Sprintf("OAUTH2_EXPIRES_IN=%d", token.ExpiresIn),
	}

	for _, line := range tokenContent {
		if _, err := file.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("failed to write token content: %w", err)
		}
	}

	r.logger.Info("OAuth2 token saved successfully to .env file")
	return nil
}

// LoadToken loads the OAuth2 token from .env file
func (r *EnvFileTokenRepository) LoadToken() (*models.OAuth2Token, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.logger.Info("Loading OAuth2 token from .env file", "file_path", r.filePath)

	file, err := os.Open(r.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("token not found: .env file does not exist")
		}
		return nil, fmt.Errorf("failed to open .env file: %w", err)
	}
	defer file.Close()

	token := &models.OAuth2Token{}
	found := make(map[string]bool)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "OAUTH2_ACCESS_TOKEN":
			token.AccessToken = value
			found["access_token"] = true
		case "OAUTH2_REFRESH_TOKEN":
			token.RefreshToken = value
			found["refresh_token"] = true
		case "OAUTH2_TOKEN_TYPE":
			token.TokenType = value
			found["token_type"] = true
		case "OAUTH2_EXPIRES_AT":
			if expiresAt, err := time.Parse(time.RFC3339, value); err == nil {
				token.ExpiresAt = expiresAt
				found["expires_at"] = true
			}
		case "OAUTH2_EXPIRES_IN":
			var expiresIn int
			if _, err := fmt.Sscanf(value, "%d", &expiresIn); err == nil {
				token.ExpiresIn = expiresIn
				found["expires_in"] = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read .env file: %w", err)
	}

	// Check if all required fields are present
	required := []string{"access_token", "refresh_token", "token_type", "expires_at"}
	for _, field := range required {
		if !found[field] {
			return nil, fmt.Errorf("incomplete token data: missing %s", field)
		}
	}

	r.logger.Info("OAuth2 token loaded successfully from .env file")
	return token, nil
}

// DeleteToken removes the OAuth2 token from .env file
func (r *EnvFileTokenRepository) DeleteToken(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("Deleting OAuth2 token from .env file", "file_path", r.filePath)

	// Read existing .env content (excluding OAuth2 tokens)
	existingContent := r.readNonTokenLines()

	// Rewrite .env file without OAuth2 tokens
	file, err := os.OpenFile(r.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open .env file: %w", err)
	}
	defer file.Close()

	for _, line := range existingContent {
		if _, err := file.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("failed to write content: %w", err)
		}
	}

	r.logger.Info("OAuth2 token deleted successfully from .env file")
	return nil
}

// readNonTokenLines reads all lines from .env file except OAuth2 token lines
func (r *EnvFileTokenRepository) readNonTokenLines() []string {
	var lines []string

	file, err := os.Open(r.filePath)
	if err != nil {
		return lines // Return empty if file doesn't exist
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		
		// Skip OAuth2 token lines
		if strings.HasPrefix(line, "OAUTH2_ACCESS_TOKEN=") ||
			strings.HasPrefix(line, "OAUTH2_REFRESH_TOKEN=") ||
			strings.HasPrefix(line, "OAUTH2_TOKEN_TYPE=") ||
			strings.HasPrefix(line, "OAUTH2_EXPIRES_AT=") ||
			strings.HasPrefix(line, "OAUTH2_EXPIRES_IN=") {
			continue
		}

		lines = append(lines, line)
	}

	return lines
}