package constants

// Pagination constants for database queries and API responses.
const (
	// DefaultPageSize is the default number of items per page for pagination.
	DefaultPageSize = 10

	// MaxRecapPageSize is the maximum page size for recap article queries.
	// Used in recap_articles_driver.go for batch operations.
	MaxRecapPageSize = 10000

	// DefaultCursorLimit is the default limit for cursor-based pagination.
	DefaultCursorLimit = 10

	// MaxCursorLimit is the maximum limit for cursor-based pagination.
	MaxCursorLimit = 100
)
