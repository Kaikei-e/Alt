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

func (u *indexArticleUsecase) Upsert(ctx context.Context, articleID, title, url, body string) error {
	// 1. Source Hash Calculation
	sourceHash := u.hasher.Compute(title, body)

	return u.txManager.RunInTx(ctx, func(ctx context.Context) error {
		// 2. Check existence
		doc, err := u.docRepo.GetByArticleID(ctx, articleID)
		if err != nil {
			return fmt.Errorf("failed to get document: %w", err)
		}

		var latestVer *domain.RagDocumentVersion
		if doc != nil && doc.CurrentVersionID != nil {
			latestVer, err = u.docRepo.GetLatestVersion(ctx, doc.ID)
			if err != nil {
				return fmt.Errorf("failed to get latest version: %w", err)
			}
		}

		// 3. Idempotency Check
		if latestVer != nil && latestVer.SourceHash == sourceHash && latestVer.URL == url && latestVer.Title == title {
			return nil
		}

		// 4. Create chunks
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

		// Embed
		if u.encoder != nil {
			embeddings, err := u.encoder.Encode(ctx, contentsToEmbed)
			// If encoder fails (and is not nil), we fail.
			if err == nil && embeddings != nil {
				if len(embeddings) != len(contentsToEmbed) {
					return fmt.Errorf("embeddings count mismatch")
				}
				for i, idx := range chunkIndicesToEmbed {
					ragChunks[idx].Embedding = pgvector.NewVector(embeddings[i])
				}
			} else if err != nil {
				return fmt.Errorf("failed to encode chunks: %w", err)
			}
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
			ChunkerVersion:  string(u.chunker.Version()),
			EmbedderVersion: "v1", // Placeholder
			CreatedAt:       now,
		}
		if latestVer != nil {
			newVer.VersionNumber = latestVer.VersionNumber + 1
			if u.encoder != nil {
				newVer.EmbedderVersion = u.encoder.Version()
			}
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

			// Map DiffEvents to RagChunkEvents
			for _, de := range diffEvents {
				rce := domain.RagChunkEvent{
					ID:        uuid.New(),
					VersionID: newVersionID,
					CreatedAt: now,
					EventType: string(de.Type),
				}

				switch de.Type {
				case domain.ChunkEventAdded:
					rce.ChunkID = chunkIDPtr(ragChunks[de.NewChunk.Ordinal].ID)
					rce.Ordinal = de.NewChunk.Ordinal
				case domain.ChunkEventUpdated:
					rce.ChunkID = chunkIDPtr(ragChunks[de.NewChunk.Ordinal].ID)
					rce.Ordinal = de.NewChunk.Ordinal
				case domain.ChunkEventUnchanged:
					rce.ChunkID = chunkIDPtr(ragChunks[de.NewChunk.Ordinal].ID)
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

// Helper to get pointer to UUID
func chunkIDPtr(id uuid.UUID) *uuid.UUID {
	return &id
}

func computeHash(content string) string {
	hashBytes := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hashBytes[:])
}
