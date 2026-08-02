package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"rag-orchestrator/internal/domain"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

type IndexArticleUsecase interface {
	// Upsert indexes an article. It is idempotent.
	Upsert(ctx context.Context, articleID, title, url, body string) error
	// Delete removes an article (soft delete logic).
	Delete(ctx context.Context, articleID string) error
}

type indexArticleUsecase struct {
	docRepo   domain.RagDocumentRepository
	chunkRepo domain.RagChunkRepository
	txManager domain.TransactionManager
	hasher    domain.SourceHashPolicy
	chunker   domain.Chunker
	encoder   domain.VectorEncoder
}

func NewIndexArticleUsecase(
	docRepo domain.RagDocumentRepository,
	chunkRepo domain.RagChunkRepository,
	txManager domain.TransactionManager,
	hasher domain.SourceHashPolicy,
	chunker domain.Chunker,
	encoder domain.VectorEncoder,
) IndexArticleUsecase {
	return &indexArticleUsecase{
		docRepo:   docRepo,
		chunkRepo: chunkRepo,
		txManager: txManager,
		hasher:    hasher,
		chunker:   chunker,
		encoder:   encoder,
	}
}

// Upsert indexes an article: (1) a read-only pre-check skips the work
// entirely when the article hasn't changed, (2) chunking, (3) embedding —
// a network call to the encoder that can take 10-60s for large articles —
// all run with NO database transaction open, and only then (4) does a
// single write transaction open to re-check idempotency and persist
// document/version/chunks/events atomically. Holding a transaction open
// across the embedder network call previously caused idle-in-transaction
// timeouts (SQLSTATE 25P03) on rag-db for the largest articles.
func (u *indexArticleUsecase) Upsert(ctx context.Context, articleID, title, url, body string) error {
	sourceHash := u.hasher.Compute(title, body)
	chunkerVersion := string(u.chunker.Version())
	embedderVersion := ""
	if u.encoder != nil {
		embedderVersion = u.encoder.Version()
	}

	// Pre-check (read-only, no transaction): skip chunking/embedding
	// entirely when the article hasn't changed. This is a fast-path only —
	// the authoritative check is repeated inside the write transaction
	// below, since state can change between this read and the transaction
	// start (e.g. a concurrent upsert of the same article committing
	// first).
	_, latestVer, err := u.loadCurrent(ctx, articleID)
	if err != nil {
		return err
	}
	if isUpToDate(latestVer, sourceHash, url, title, chunkerVersion, embedderVersion) {
		return nil
	}

	// Create chunks (CPU-only, no I/O).
	chunks, err := u.chunker.Chunk(body)
	if err != nil {
		return fmt.Errorf("failed to chunk body: %w", err)
	}

	now := time.Now()
	newVersionID := uuid.New()

	// Map domain.Chunk to domain.RagChunk
	var ragChunks []domain.RagChunk
	var contentsToEmbed []string
	var chunkIndicesToEmbed []int

	for i, c := range chunks {
		ragChunk := domain.RagChunk{
			ID:        uuid.New(),
			VersionID: newVersionID,
			Ordinal:   c.Ordinal,
			Content:   c.Content,
			CreatedAt: now,
		}
		ragChunks = append(ragChunks, ragChunk)
		contentsToEmbed = append(contentsToEmbed, c.Content)
		chunkIndicesToEmbed = append(chunkIndicesToEmbed, i)
	}

	// Embed — the network call to the encoder — happens here, before any
	// transaction is opened.
	if u.encoder != nil && len(contentsToEmbed) > 0 {
		embeddings, err := u.encoder.Encode(ctx, contentsToEmbed)
		if err != nil {
			return fmt.Errorf("failed to encode chunks: %w", err)
		}
		// A nil/short embeddings slice with no error is indistinguishable from
		// silent encoder failure; treat it as an explicit error rather than
		// storing chunks without embeddings.
		if len(embeddings) != len(contentsToEmbed) {
			return fmt.Errorf("embeddings count mismatch: got %d, want %d", len(embeddings), len(contentsToEmbed))
		}
		for i, idx := range chunkIndicesToEmbed {
			// Width, not just count: a wrong model behind the embedder URL
			// otherwise surfaces as a raw pgvector error from inside the write
			// transaction, and a same-width wrong model not at all.
			if len(embeddings[i]) != domain.EmbeddingDimension {
				return fmt.Errorf("embedding dimension mismatch from embedder %q: got %d, want %d",
					embedderVersion, len(embeddings[i]), domain.EmbeddingDimension)
			}
			ragChunks[idx].Embedding = pgvector.NewVector(embeddings[i])
		}
	}

	return u.txManager.RunInTx(ctx, func(ctx context.Context) error {
		// Re-check inside the transaction: another upsert of the same
		// article may have committed between the pre-check above and this
		// transaction beginning. This re-check — not the pre-check — is
		// what keeps Upsert idempotent under concurrent callers. A losing
		// writer that still gets past this check will hit the DB's unique
		// constraints (rag_documents.article_id;
		// rag_document_versions(document_id, version_number)) on insert,
		// which the HTTP handler already reports as 409 Conflict
		// (isDuplicateKeyError in rag_http/handler.go).
		doc, latestVer, err := u.loadCurrent(ctx, articleID)
		if err != nil {
			return err
		}
		if isUpToDate(latestVer, sourceHash, url, title, chunkerVersion, embedderVersion) {
			return nil
		}

		// Insert Document if new
		if doc == nil {
			doc = &domain.RagDocument{
				ID:        uuid.New(),
				ArticleID: articleID,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := u.docRepo.CreateDocument(ctx, doc); err != nil {
				return fmt.Errorf("failed to create document: %w", err)
			}
		}

		// Insert Version
		newVer := &domain.RagDocumentVersion{
			ID:              newVersionID,
			DocumentID:      doc.ID,
			VersionNumber:   1,
			Title:           title,
			URL:             url,
			SourceHash:      sourceHash,
			ChunkerVersion:  chunkerVersion,
			EmbedderVersion: embedderVersion,
			CreatedAt:       now,
		}
		if latestVer != nil {
			newVer.VersionNumber = latestVer.VersionNumber + 1
		}
		if err := u.docRepo.CreateVersion(ctx, newVer); err != nil {
			return fmt.Errorf("failed to create version: %w", err)
		}

		// Insert Chunks
		if err := u.chunkRepo.BulkInsertChunks(ctx, ragChunks); err != nil {
			return fmt.Errorf("failed to insert chunks: %w", err)
		}

		// Compute Diff Events
		var chunkEvents []domain.RagChunkEvent

		if latestVer == nil {
			// All Added
			for _, rc := range ragChunks {
				id := rc.ID
				chunkEvents = append(chunkEvents, domain.RagChunkEvent{
					ID:        uuid.New(),
					VersionID: newVersionID,
					ChunkID:   &id,
					Ordinal:   rc.Ordinal,
					EventType: "added",
					CreatedAt: now,
				})
			}
		} else {
			// Fetch old chunks and compute diff
			oldRagChunks, err := u.chunkRepo.GetChunksByVersionID(ctx, latestVer.ID)
			if err != nil {
				return fmt.Errorf("failed to fetch old chunks: %w", err)
			}

			var oldChunks []domain.Chunk
			oldChunkMap := make(map[int]uuid.UUID) // Ordinal -> ID

			for _, rc := range oldRagChunks {
				oldChunks = append(oldChunks, domain.Chunk{
					Ordinal: rc.Ordinal,
					Content: rc.Content,
					Hash:    computeHash(rc.Content),
				})
				oldChunkMap[rc.Ordinal] = rc.ID
			}

			// Run Diff
			diffEvents := domain.DiffChunks(oldChunks, chunks)

			ordinalToIdx := make(map[int]int, len(ragChunks))
			for i, rc := range ragChunks {
				ordinalToIdx[rc.Ordinal] = i
			}

			// Map DiffEvents to RagChunkEvents
			for _, de := range diffEvents {
				rce := domain.RagChunkEvent{
					ID:        uuid.New(),
					VersionID: newVersionID,
					CreatedAt: now,
					EventType: string(de.Type),
				}

				switch de.Type {
				case domain.ChunkEventAdded, domain.ChunkEventUpdated, domain.ChunkEventUnchanged:
					idx, ok := ordinalToIdx[de.NewChunk.Ordinal]
					if !ok {
						// Ordinal may be non-contiguous; skip rather than panic on slice index.
						continue
					}
					rce.ChunkID = chunkIDPtr(ragChunks[idx].ID)
					rce.Ordinal = de.NewChunk.Ordinal
				case domain.ChunkEventDeleted:
					if oldID, ok := oldChunkMap[de.OldChunk.Ordinal]; ok {
						rce.ChunkID = chunkIDPtr(oldID)
					}
					rce.Ordinal = de.OldChunk.Ordinal
				}

				chunkEvents = append(chunkEvents, rce)
			}
		}

		if err := u.chunkRepo.InsertEvents(ctx, chunkEvents); err != nil {
			return fmt.Errorf("failed to insert events: %w", err)
		}

		// Update Current Version
		if err := u.docRepo.UpdateCurrentVersion(ctx, doc.ID, newVersionID); err != nil {
			return fmt.Errorf("failed to update current version: %w", err)
		}

		return nil
	})
}

func (u *indexArticleUsecase) Delete(ctx context.Context, articleID string) error {
	return u.txManager.RunInTx(ctx, func(ctx context.Context) error {
		doc, err := u.docRepo.GetByArticleID(ctx, articleID)
		if err != nil {
			return fmt.Errorf("failed to get document: %w", err)
		}
		if doc == nil || doc.CurrentVersionID == nil {
			return nil // Already checked/deleted or not found
		}

		latestVer, err := u.docRepo.GetLatestVersion(ctx, doc.ID)
		if err != nil {
			return fmt.Errorf("failed to get latest version: %w", err)
		}
		// Idempotent: already tombstoned — do not stack empty tombstone versions.
		if latestVer != nil && latestVer.ChunkerVersion == "tombstone" {
			return nil
		}

		oldRagChunks, err := u.chunkRepo.GetChunksByVersionID(ctx, latestVer.ID)
		if err != nil {
			return fmt.Errorf("failed to fetch old chunks: %w", err)
		}

		now := time.Now()
		newVersionID := uuid.New()

		// Create Tombstone Version (version with empty hash/content)
		newVer := &domain.RagDocumentVersion{
			ID:              newVersionID,
			DocumentID:      doc.ID,
			VersionNumber:   latestVer.VersionNumber + 1,
			SourceHash:      "", // Empty denotes deleted? Or specific value.
			ChunkerVersion:  "tombstone",
			EmbedderVersion: "tombstone",
			CreatedAt:       now,
		}
		if err := u.docRepo.CreateVersion(ctx, newVer); err != nil {
			return fmt.Errorf("failed to create tombstone version: %w", err)
		}

		// Create 'deleted' events for all old chunks
		var events []domain.RagChunkEvent
		for _, rc := range oldRagChunks {
			events = append(events, domain.RagChunkEvent{
				ID:        uuid.New(),
				VersionID: newVersionID,
				ChunkID:   chunkIDPtr(rc.ID),
				Ordinal:   rc.Ordinal,
				EventType: "deleted",
				CreatedAt: now,
			})
		}

		if err := u.chunkRepo.InsertEvents(ctx, events); err != nil {
			return fmt.Errorf("failed to insert delete events: %w", err)
		}

		// Update current version
		if err := u.docRepo.UpdateCurrentVersion(ctx, doc.ID, newVersionID); err != nil {
			return fmt.Errorf("failed to update current version: %w", err)
		}

		return nil
	})
}

// loadCurrent loads the document and its latest version for articleID.
// Returns (nil, nil, nil) if the document doesn't exist yet, and
// (doc, nil, nil) if it exists but has no version. Used both by the
// pre-check (outside any transaction) and the re-check (inside the write
// transaction) in Upsert.
func (u *indexArticleUsecase) loadCurrent(ctx context.Context, articleID string) (*domain.RagDocument, *domain.RagDocumentVersion, error) {
	doc, err := u.docRepo.GetByArticleID(ctx, articleID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get document: %w", err)
	}
	if doc == nil || doc.CurrentVersionID == nil {
		return doc, nil, nil
	}
	latestVer, err := u.docRepo.GetLatestVersion(ctx, doc.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get latest version: %w", err)
	}
	return doc, latestVer, nil
}

// isUpToDate reports whether latestVer already reflects sourceHash/url/title
// under chunkerVersion and embedderVersion, i.e. re-indexing would be a no-op.
// Both derivation versions participate: a chunker change (e.g. v8->v9 HTML
// sanitization) invalidates the chunk text, and an embedder change invalidates
// the vectors derived from it. A version row that predates the current
// embedder — including one that never recorded it, which reads back as the
// empty string — is stale by the same comparison.
func isUpToDate(latestVer *domain.RagDocumentVersion, sourceHash, url, title, chunkerVersion, embedderVersion string) bool {
	return latestVer != nil &&
		latestVer.SourceHash == sourceHash &&
		latestVer.URL == url &&
		latestVer.Title == title &&
		latestVer.ChunkerVersion == chunkerVersion &&
		latestVer.EmbedderVersion == embedderVersion
}

// Helper to get pointer to UUID
func chunkIDPtr(id uuid.UUID) *uuid.UUID {
	return &id
}

func computeHash(content string) string {
	hashBytes := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hashBytes[:])
}
