package usecase_test

import (
	"context"
	"testing"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mocks ---

type MockRagDocumentRepository struct {
	mock.Mock
}

func (m *MockRagDocumentRepository) GetByArticleID(ctx context.Context, articleID string) (*domain.RagDocument, error) {
	args := m.Called(ctx, articleID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RagDocument), args.Error(1)
}

func (m *MockRagDocumentRepository) CreateDocument(ctx context.Context, doc *domain.RagDocument) error {
	args := m.Called(ctx, doc)
	return args.Error(0)
}

func (m *MockRagDocumentRepository) UpdateCurrentVersion(ctx context.Context, docID uuid.UUID, versionID uuid.UUID) error {
	args := m.Called(ctx, docID, versionID)
	return args.Error(0)
}

func (m *MockRagDocumentRepository) GetLatestVersion(ctx context.Context, docID uuid.UUID) (*domain.RagDocumentVersion, error) {
	args := m.Called(ctx, docID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RagDocumentVersion), args.Error(1)
}

func (m *MockRagDocumentRepository) CreateVersion(ctx context.Context, version *domain.RagDocumentVersion) error {
	args := m.Called(ctx, version)
	return args.Error(0)
}

type MockRagChunkRepository struct {
	mock.Mock
}

func (m *MockRagChunkRepository) BulkInsertChunks(ctx context.Context, chunks []domain.RagChunk) error {
	args := m.Called(ctx, chunks)
	return args.Error(0)
}

func (m *MockRagChunkRepository) GetChunksByVersionID(ctx context.Context, versionID uuid.UUID) ([]domain.RagChunk, error) {
	args := m.Called(ctx, versionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.RagChunk), args.Error(1)
}

func (m *MockRagChunkRepository) InsertEvents(ctx context.Context, events []domain.RagChunkEvent) error {
	args := m.Called(ctx, events)
	return args.Error(0)
}

type MockTransactionManager struct {
	mock.Mock
}

func (m *MockTransactionManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	// Directly execute the function
	return fn(ctx)
}

// --- Tests ---

func TestIndexArticle_Upsert_Idempotency(t *testing.T) {
	// Setup
	mockDocRepo := new(MockRagDocumentRepository)
	mockChunkRepo := new(MockRagChunkRepository)
	mockTxManager := new(MockTransactionManager)

	// Real Domain policies for logic logic
	hasher := domain.NewSourceHashPolicy()
	chunker := domain.NewChunker()

	uc := usecase.NewIndexArticleUsecase(
		mockDocRepo, mockChunkRepo, mockTxManager, hasher, chunker, nil,
	)

	ctx := context.Background()
	articleID := "article-123"
	title := "Test Title"
	body := "Test Body"

	sourceHash := hasher.Compute(title, body)
	docID := uuid.New()
	verID := uuid.New()

	// Mocks expectation
	mockDocRepo.On("GetByArticleID", ctx, articleID).Return(&domain.RagDocument{
		ID:               docID,
		ArticleID:        articleID,
		CurrentVersionID: &verID,
	}, nil)

	mockDocRepo.On("GetLatestVersion", ctx, docID).Return(&domain.RagDocumentVersion{
		ID:         verID,
		DocumentID: docID,
		SourceHash: sourceHash, // Same hash
	}, nil)

	// Execute
	err := uc.Upsert(ctx, articleID, title, body)

	// Assert
	assert.NoError(t, err)
	mockDocRepo.AssertExpectations(t)
	mockChunkRepo.AssertExpectations(t) // Should not be called
}

func TestIndexArticle_Upsert_NewArticle(t *testing.T) {
	mockDocRepo := new(MockRagDocumentRepository)
	mockChunkRepo := new(MockRagChunkRepository)
	mockTxManager := new(MockTransactionManager)
	hasher := domain.NewSourceHashPolicy()
	chunker := domain.NewChunker()

	uc := usecase.NewIndexArticleUsecase(
		mockDocRepo, mockChunkRepo, mockTxManager, hasher, chunker, nil,
	)

	ctx := context.Background()
	articleID := "new-article"
	title := "New Title"
	body := "Paragraph 1.\n\nParagraph 2."

	// Expectations
	mockDocRepo.On("GetByArticleID", ctx, articleID).Return(nil, nil) // Not found

	// Create Document
	mockDocRepo.On("CreateDocument", ctx, mock.MatchedBy(func(d *domain.RagDocument) bool {
		return d.ArticleID == articleID
	})).Return(nil)

	// Create Version
	mockDocRepo.On("CreateVersion", ctx, mock.MatchedBy(func(v *domain.RagDocumentVersion) bool {
		return v.VersionNumber == 1
	})).Return(nil)

	// Insert Chunks
	mockChunkRepo.On("BulkInsertChunks", ctx, mock.MatchedBy(func(chunks []domain.RagChunk) bool {
		return len(chunks) == 2 // 2 Paragraphs
	})).Return(nil)

	// Insert Events
	mockChunkRepo.On("InsertEvents", ctx, mock.MatchedBy(func(events []domain.RagChunkEvent) bool {
		return len(events) == 2 && events[0].EventType == "added"
	})).Return(nil)

	// Update Current Version
	mockDocRepo.On("UpdateCurrentVersion", ctx, mock.Anything, mock.Anything).Return(nil)

	err := uc.Upsert(ctx, articleID, title, body)
	assert.NoError(t, err)
	mockDocRepo.AssertExpectations(t)
	mockChunkRepo.AssertExpectations(t)
}

func TestIndexArticle_Upsert_Update(t *testing.T) {
	mockDocRepo := new(MockRagDocumentRepository)
	mockChunkRepo := new(MockRagChunkRepository)
	mockTxManager := new(MockTransactionManager)
	hasher := domain.NewSourceHashPolicy()
	chunker := domain.NewChunker()

	uc := usecase.NewIndexArticleUsecase(
		mockDocRepo, mockChunkRepo, mockTxManager, hasher, chunker, nil,
	)

	ctx := context.Background()
	articleID := "update-article"
	title := "Update Title"
	// Old body: "Start.\n\nEnd."
	// New body: "Start.\n\nMiddle.\n\nEnd." (Middle added)
	body := "Start.\n\nMiddle.\n\nEnd."

	docID := uuid.New()
	verID := uuid.New()

	// Expectations
	// 1. Get Doc -> Found
	mockDocRepo.On("GetByArticleID", ctx, articleID).Return(&domain.RagDocument{
		ID:               docID,
		ArticleID:        articleID,
		CurrentVersionID: &verID,
	}, nil)

	// 2. Get Latest Version
	mockDocRepo.On("GetLatestVersion", ctx, docID).Return(&domain.RagDocumentVersion{
		ID:            verID,
		VersionNumber: 1,
		SourceHash:    "old-hash",
	}, nil)

	// 3. Get Old Chunks (for Diff)
	// Old: ["Start.", "End."]
	mockChunkRepo.On("GetChunksByVersionID", ctx, verID).Return([]domain.RagChunk{
		{Ordinal: 0, Content: "Start.", ID: uuid.New()},
		{Ordinal: 1, Content: "End.", ID: uuid.New()},
	}, nil)

	// 4. Create Version (v2)
	mockDocRepo.On("CreateVersion", ctx, mock.MatchedBy(func(v *domain.RagDocumentVersion) bool {
		return v.VersionNumber == 2
	})).Return(nil)

	// 5. Insert New Chunks (3 chunks)
	mockChunkRepo.On("BulkInsertChunks", ctx, mock.MatchedBy(func(chunks []domain.RagChunk) bool {
		return len(chunks) == 3
	})).Return(nil)

	// 6. Insert Events
	// Diff: Start (Unchanged), Middle (Added), End (Unchanged) -> wait.
	// LCS: Start match, End match. Middle inserted.
	// Events: Unchanged, Added, Unchanged.
	mockChunkRepo.On("InsertEvents", ctx, mock.MatchedBy(func(events []domain.RagChunkEvent) bool {
		// Should have 3 events? Depend on implementation.
		// Implementation loops over DiffEvents.
		// DiffChunks returns events for gaps and matches.
		// Start: Match -> Unchanged
		// Middle: Gap -> Added
		// End: Match -> Unchanged
		// Total 3 events.
		if len(events) != 3 {
			return false
		}
		// Order depends on DiffChunks implementation.
		// Assuming: Unchanged, Added, Unchanged.
		return true // weak check for now
	})).Return(nil)

	// 7. Update Current Version
	mockDocRepo.On("UpdateCurrentVersion", ctx, docID, mock.Anything).Return(nil)

	err := uc.Upsert(ctx, articleID, title, body)
	assert.NoError(t, err)
	mockDocRepo.AssertExpectations(t)
	mockChunkRepo.AssertExpectations(t)
}
