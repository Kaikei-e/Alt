package alt_db

import (
	"context"
	"fmt"
	"time"
)

// SaveDeclinedDomain saves a domain to the declined_domains table
func (r *AltDBRepository) SaveDeclinedDomain(ctx context.Context, userID, domain string) error {
	query := `
		INSERT INTO declined_domains (user_id, domain, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`
	_, err := r.pool.Exec(ctx, query, userID, domain, time.Now())
	if err != nil {
		return fmt.Errorf("failed to save declined domain: %w", err)
	}
	return nil
}

// IsDomainDeclined checks if a domain is in the declined_domains table for a user
func (r *AltDBRepository) IsDomainDeclined(ctx context.Context, userID, domain string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM declined_domains
			WHERE user_id = $1 AND domain = $2
		)
	`
	var exists bool
	err := r.pool.QueryRow(ctx, query, userID, domain).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check declined domain: %w", err)
	}
	return exists, nil
}
