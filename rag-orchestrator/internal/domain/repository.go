package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

// RagDocument represents a document in the system.
type RagDocument struct {
	ID               uuid.UUID
	ArticleID        string
	CurrentVersionID *uuid.UUID // Can be nil if no version exists yet
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// RagDocumentVersion represents an immutable version of a document.
type RagDocumentVersion struct {
	ID              uuid.UUID
	DocumentID      uuid.UUID
	VersionNumber   int
	SourceHash      string
	ChunkerVersion  string
	EmbedderVersion string
	CreatedAt       time.Time
}

// RagChunk represents a persistable chunk.
type RagChunk struct {
	ID        uuid.UUID
	VersionID uuid.UUID
	Ordinal   int
	Content   string
	Embedding pgvector.Vector // Using pgvector-go type
	CreatedAt time.Time
}

// RagChunkEvent represents a persistable chunk event.
type RagChunkEvent struct {
	ID        uuid.UUID
	VersionID uuid.UUID
	ChunkID   *uuid.UUID // Nullable
	Ordinal   int
	EventType string // "added", "updated", "deleted", "unchanged"
	Metadata  map[string]interface{}
	CreatedAt time.Time
}

// RagDocumentRepository defines the operations for managing documents and their versions.
type RagDocumentRepository interface {
	// GetByArticleID retrieves a document by its Article ID.
	// Returns nil, nil if not found.
	GetByArticleID(ctx context.Context, articleID string) (*RagDocument, error)

	// CreateDocument creates a new document.
	CreateDocument(ctx context.Context, doc *RagDocument) error

	// UpdateCurrentVersion updates the current_version_id of a document.
	UpdateCurrentVersion(ctx context.Context, docID uuid.UUID, versionID uuid.UUID) error

	// GetLatestVersion retrieves the latest version info for a document.
	// Returns nil, nil if no version exists.
	GetLatestVersion(ctx context.Context, docID uuid.UUID) (*RagDocumentVersion, error)

	// CreateVersion creates a new document version.
	CreateVersion(ctx context.Context, version *RagDocumentVersion) error
}

// RagChunkRepository defines the operations for managing chunks and events.
type RagChunkRepository interface {
	// BulkInsertChunks inserts multiple chunks.
	BulkInsertChunks(ctx context.Context, chunks []RagChunk) error

	// GetChunksByVersionID retrieves chunks for a specific version, ordered by ordinal.
	GetChunksByVersionID(ctx context.Context, versionID uuid.UUID) ([]RagChunk, error)

	// InsertEvents inserts multiple chunk events.
	InsertEvents(ctx context.Context, events []RagChunkEvent) error

	// Search performs a vector search for chunks.
	// candidateArticleIDs: if not empty, filter chunks to these articles.
	Search(ctx context.Context, queryVector []float32, candidateArticleIDs []string, limit int) ([]SearchResult, error)
}

// SearchResult represents a chunk found via vector search, including its similarity score.
type SearchResult struct {
	Chunk           RagChunk
	Score           float32
	ArticleID       string
	DocumentVersion int
}

// TransactionManager defines the interface for handling database transactions.
type TransactionManager interface {
	// RunInTx executes the given function within a transaction.
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
