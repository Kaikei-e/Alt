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
	Title           string
	URL             string
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

	// GetVersionByID retrieves a document version by its ID.
	// Returns nil, nil if not found.
	GetVersionByID(ctx context.Context, versionID uuid.UUID) (*RagDocumentVersion, error)

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

	// Search performs a vector search across all chunks (Augur use case).
	// Uses Two-Stage Search for HNSW index efficiency.
	Search(ctx context.Context, queryVector []float32, limit int) ([]SearchResult, error)

	// SearchWithinArticles performs a vector search within specific articles (Morning Letter use case).
	// Uses pre-filtering by article IDs before vector search.
	// articleIDs must not be empty.
	SearchWithinArticles(ctx context.Context, queryVector []float32, articleIDs []string, limit int) ([]SearchResult, error)
}

// ScoreKind names the space a Score value lives in.
//
// A float32 relevance score is meaningless without it: a cosine similarity of
// 0.9 and a BM25 score of 0.9 and an RRF score of 0.9 describe three unrelated
// degrees of relevance, and the retrieval pipeline produces all three depending
// on which arms were available. Comparing one against thresholds calibrated for
// another is what turned an embedder outage into a 58.3% empty-answer rate
// (docs/review/augur-fallback-rate-regression-analysis-2026-04-11.md): promoted
// BM25 scores were read as cross-encoder scores and failed a 0.25 gate that had
// nothing to do with them.
type ScoreKind string

const (
	// ScoreKindUnknown marks a score that did not come from the retrieval
	// pipeline — a hand-built context, or a fixed placeholder such as the
	// uniform 1.0 an article-scoped read assigns. It is never comparable to a
	// calibrated threshold.
	ScoreKindUnknown ScoreKind = ""
	// ScoreKindVector is cosine similarity (1 - distance) from pgvector.
	ScoreKindVector ScoreKind = "vector"
	// ScoreKindBM25 is a lexical relevance score from the search index. Its
	// range depends on corpus statistics and it may be absent entirely (the
	// search-indexer response carries none today, leaving every hit at 0).
	ScoreKindBM25 ScoreKind = "bm25"
	// ScoreKindRRF is a Reciprocal Rank Fusion score, sum of 1/(k+rank). For
	// the standard k=60 it sits around 0.016-0.033 however relevant the hit.
	ScoreKindRRF ScoreKind = "rrf"
	// ScoreKindRerank is a cross-encoder relevance score. This is the only
	// space the quality thresholds are calibrated against.
	ScoreKindRerank ScoreKind = "rerank"
)

// Calibrated reports whether a score in this space may be compared against the
// retrieval quality thresholds. Only the cross-encoder produces such scores;
// everything else is a ranking signal, useful for ordering and useless as an
// absolute judgement of relevance.
func (k ScoreKind) Calibrated() bool { return k == ScoreKindRerank }

// SearchResult represents a chunk found via vector search, including its similarity score.
type SearchResult struct {
	Chunk RagChunk
	Score float32
	// ScoreKind declares which space Score lives in. Every producer of a
	// SearchResult must set it; an unset kind means the score cannot be gated.
	ScoreKind       ScoreKind
	ArticleID       string
	Title           string
	URL             string
	DocumentVersion int
}

// HybridSearcher performs in-database hybrid search (dense vector + sparse tsvector)
// with Reciprocal Rank Fusion (RRF). Replaces application-level BM25 + vector fusion.
type HybridSearcher interface {
	// HybridSearch performs a combined vector + full-text search with RRF fusion.
	HybridSearch(ctx context.Context, queryVector []float32, queryText string, limit int) ([]SearchResult, error)

	// SearchNeighbors finds articles semantically and lexically near a seed set
	// using the same hybrid (vector + full-text) RRF pipeline as HybridSearch,
	// but excludes articles whose ArticleID appears in seedArticleIDs. Used to
	// build the inline-projected "related" snapshot for Ask Augur citations.
	// queryVector may be empty; in that case the fallback is text-only matching
	// over queryText.
	SearchNeighbors(ctx context.Context, queryVector []float32, queryText string, seedArticleIDs []string, limit int) ([]SearchResult, error)
}

// TransactionManager defines the interface for handling database transactions.
type TransactionManager interface {
	// RunInTx executes the given function within a transaction.
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// RagJob represents a background job.
type RagJob struct {
	ID           uuid.UUID
	JobType      string
	Payload      map[string]interface{} // JSONB
	Status       string                 // "new", "processing", "completed", "failed"
	ErrorMessage *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// RagJobRepository defines the operations for managing background jobs.
type RagJobRepository interface {
	// Enqueue adds a new job to the queue.
	Enqueue(ctx context.Context, job *RagJob) error

	// AcquireNextJob retrieves the next available 'new' job and locks it (SKIP LOCKED).
	// Returns nil, nil if no job is available.
	AcquireNextJob(ctx context.Context) (*RagJob, error)

	// UpdateStatus updates the status and error message of a job.
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, errorMessage *string) error
}
