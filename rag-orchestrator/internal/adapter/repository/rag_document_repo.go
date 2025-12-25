package repository

import (
	"context"
	"errors"
	"fmt"
	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ragDocumentRepository struct {
	pool *pgxpool.Pool
}

// NewRagDocumentRepository creates a new RagDocumentRepository.
func NewRagDocumentRepository(pool *pgxpool.Pool) domain.RagDocumentRepository {
	return &ragDocumentRepository{pool: pool}
}

func (r *ragDocumentRepository) getExecutor(ctx context.Context) interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
} {
	tx := ExtractTx(ctx)
	if tx != nil {
		return tx
	}
	return r.pool
}

func (r *ragDocumentRepository) GetByArticleID(ctx context.Context, articleID string) (*domain.RagDocument, error) {
	query := `
		SELECT id, article_id, current_version_id, created_at, updated_at
		FROM rag_documents
		WHERE article_id = $1
	`
	row := r.getExecutor(ctx).QueryRow(ctx, query, articleID)

	var doc domain.RagDocument
	err := row.Scan(&doc.ID, &doc.ArticleID, &doc.CurrentVersionID, &doc.CreatedAt, &doc.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan document: %w", err)
	}
	return &doc, nil
}

func (r *ragDocumentRepository) CreateDocument(ctx context.Context, doc *domain.RagDocument) error {
	query := `
		INSERT INTO rag_documents (id, article_id, current_version_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.getExecutor(ctx).Exec(ctx, query, doc.ID, doc.ArticleID, doc.CurrentVersionID, doc.CreatedAt, doc.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert document: %w", err)
	}
	return nil
}

func (r *ragDocumentRepository) UpdateCurrentVersion(ctx context.Context, docID uuid.UUID, versionID uuid.UUID) error {
	query := `
		UPDATE rag_documents
		SET current_version_id = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.getExecutor(ctx).Exec(ctx, query, versionID, docID)
	if err != nil {
		return fmt.Errorf("failed to update current version: %w", err)
	}
	return nil
}

func (r *ragDocumentRepository) GetLatestVersion(ctx context.Context, docID uuid.UUID) (*domain.RagDocumentVersion, error) {
	query := `
		SELECT id, document_id, version_number, title, url, source_hash, chunker_version, embedder_version, created_at
		FROM rag_document_versions
		WHERE document_id = $1
		ORDER BY version_number DESC
		LIMIT 1
	`
	row := r.getExecutor(ctx).QueryRow(ctx, query, docID)

	var ver domain.RagDocumentVersion
	var title, url pgtype.Text
	err := row.Scan(&ver.ID, &ver.DocumentID, &ver.VersionNumber, &title, &url, &ver.SourceHash, &ver.ChunkerVersion, &ver.EmbedderVersion, &ver.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan version: %w", err)
	}

	ver.Title = title.String
	ver.URL = url.String

	return &ver, nil
}

func (r *ragDocumentRepository) CreateVersion(ctx context.Context, version *domain.RagDocumentVersion) error {
	query := `
		INSERT INTO rag_document_versions (id, document_id, version_number, title, url, source_hash, chunker_version, embedder_version, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.getExecutor(ctx).Exec(ctx, query, version.ID, version.DocumentID, version.VersionNumber, version.Title, version.URL, version.SourceHash, version.ChunkerVersion, version.EmbedderVersion, version.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert version: %w", err)
	}
	return nil
}
