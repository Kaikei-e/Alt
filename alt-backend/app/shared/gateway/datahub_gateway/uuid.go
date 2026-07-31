package datahub_gateway

import (
	"fmt"

	"github.com/google/uuid"
)

// parseUUID turns a wire string into a uuid.UUID. An empty string is the zero
// UUID rather than an error: SaveScrapingDomain sends a request whose id is
// unset when the row does not exist yet, and the provider assigns one.
func parseUUID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse uuid %q: %w", s, err)
	}
	return id, nil
}
