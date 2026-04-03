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

func (m *MockRagDocumentRepository) GetVersionByID(ctx context.Context, versionID uuid.UUID) (*domain.RagDocumentVersion, error) {
	args := m.Called(ctx, versionID)
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

func (m *MockRagChunkRepository) Search(ctx context.Context, queryVector []float32, limit int) ([]domain.SearchResult, error) {
	args := m.Called(ctx, queryVector, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.SearchResult), args.Error(1)
}

func (m *MockRagChunkRepository) SearchWithinArticles(ctx context.Context, queryVector []float32, articleIDs []string, limit int) ([]domain.SearchResult, error) {
	args := m.Called(ctx, queryVector, articleIDs, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.SearchResult), args.Error(1)
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
		ID:             verID,
		DocumentID:     docID,
		SourceHash:     sourceHash, // Same hash
		Title:          title,
		ChunkerVersion: string(domain.ChunkerVersionV9), // Same chunker version
	}, nil)

	// Execute
	err := uc.Upsert(ctx, articleID, title, "", body)

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
	// Note: Short paragraphs (< 80 chars) are merged by Chunker v8
	mockChunkRepo.On("BulkInsertChunks", ctx, mock.MatchedBy(func(chunks []domain.RagChunk) bool {
		return len(chunks) == 1 // Merged into 1 chunk due to MinChunkLength
	})).Return(nil)

	// Insert Events
	mockChunkRepo.On("InsertEvents", ctx, mock.MatchedBy(func(events []domain.RagChunkEvent) bool {
		return len(events) == 1 && events[0].EventType == "added"
	})).Return(nil)

	// Update Current Version
	mockDocRepo.On("UpdateCurrentVersion", ctx, mock.Anything, mock.Anything).Return(nil)

	err := uc.Upsert(ctx, articleID, title, "", body)
	assert.NoError(t, err)
	mockDocRepo.AssertExpectations(t)
	mockChunkRepo.AssertExpectations(t)
}

func TestIndexArticle_Upsert_ReindexOnChunkerVersionChange(t *testing.T) {
	// When SourceHash/Title/URL match but ChunkerVersion is old (v8),
	// the article must be re-indexed with the new chunker (v9).
	mockDocRepo := new(MockRagDocumentRepository)
	mockChunkRepo := new(MockRagChunkRepository)
	mockTxManager := new(MockTransactionManager)
	hasher := domain.NewSourceHashPolicy()
	chunker := domain.NewChunker() // Returns V9

	uc := usecase.NewIndexArticleUsecase(
		mockDocRepo, mockChunkRepo, mockTxManager, hasher, chunker, nil,
	)

	ctx := context.Background()
	articleID := "reindex-article"
	title := "Reindex Title"
	body := "Body text for reindex."

	sourceHash := hasher.Compute(title, body)
	docID := uuid.New()
	verID := uuid.New()

	mockDocRepo.On("GetByArticleID", ctx, articleID).Return(&domain.RagDocument{
		ID:               docID,
		ArticleID:        articleID,
		CurrentVersionID: &verID,
	}, nil)

	// Latest version has same hash but OLD chunker version (v8)
	mockDocRepo.On("GetLatestVersion", ctx, docID).Return(&domain.RagDocumentVersion{
		ID:             verID,
		DocumentID:     docID,
		VersionNumber:  1,
		SourceHash:     sourceHash, // Same content hash
		Title:          title,
		ChunkerVersion: string(domain.ChunkerVersionV8), // OLD version
	}, nil)

	// Because chunker version differs, re-indexing should happen:
	mockChunkRepo.On("GetChunksByVersionID", ctx, verID).Return([]domain.RagChunk{
		{Ordinal: 0, Content: "Body text for reindex.", ID: uuid.New()},
	}, nil)
	mockDocRepo.On("CreateVersion", ctx, mock.MatchedBy(func(v *domain.RagDocumentVersion) bool {
		return v.VersionNumber == 2 && v.ChunkerVersion == string(domain.ChunkerVersionV9)
	})).Return(nil)
	mockChunkRepo.On("BulkInsertChunks", ctx, mock.Anything).Return(nil)
	mockChunkRepo.On("InsertEvents", ctx, mock.Anything).Return(nil)
	mockDocRepo.On("UpdateCurrentVersion", ctx, docID, mock.Anything).Return(nil)

	err := uc.Upsert(ctx, articleID, title, "", body)
	assert.NoError(t, err)

	// Verify that CreateVersion was called (i.e., not skipped by idempotency)
	mockDocRepo.AssertCalled(t, "CreateVersion", ctx, mock.Anything)
	mockChunkRepo.AssertCalled(t, "BulkInsertChunks", ctx, mock.Anything)
}

func TestIndexArticle_Upsert_HTMLBodyProducesCleanChunks(t *testing.T) {
	// When body contains HTML, chunks should have stripped text (via sanitizer in chunker V9).
	mockDocRepo := new(MockRagDocumentRepository)
	mockChunkRepo := new(MockRagChunkRepository)
	mockTxManager := new(MockTransactionManager)
	hasher := domain.NewSourceHashPolicy()
	chunker := domain.NewChunker()

	uc := usecase.NewIndexArticleUsecase(
		mockDocRepo, mockChunkRepo, mockTxManager, hasher, chunker, nil,
	)

	ctx := context.Background()
	articleID := "html-article"
	title := "HTML Article"
	body := `<div><p>記事の本文テキストです。十分な長さを持つ文章で、HTMLタグが除去されていることを確認します。</p>` +
		`<script>alert('xss')</script>` +
		`<p>2番目の段落の本文です。こちらも十分な長さを持っています。チャンク分割の確認用テキストです。</p></div>`

	mockDocRepo.On("GetByArticleID", ctx, articleID).Return(nil, nil)
	mockDocRepo.On("CreateDocument", ctx, mock.Anything).Return(nil)
	mockDocRepo.On("CreateVersion", ctx, mock.Anything).Return(nil)

	// Capture the chunks to verify they are HTML-free
	var capturedChunks []domain.RagChunk
	mockChunkRepo.On("BulkInsertChunks", ctx, mock.MatchedBy(func(chunks []domain.RagChunk) bool {
		capturedChunks = chunks
		return true
	})).Return(nil)
	mockChunkRepo.On("InsertEvents", ctx, mock.Anything).Return(nil)
	mockDocRepo.On("UpdateCurrentVersion", ctx, mock.Anything, mock.Anything).Return(nil)

	err := uc.Upsert(ctx, articleID, title, "", body)
	assert.NoError(t, err)

	// All chunks must be HTML-free
	for _, chunk := range capturedChunks {
		assert.NotContains(t, chunk.Content, "<div>")
		assert.NotContains(t, chunk.Content, "<p>")
		assert.NotContains(t, chunk.Content, "<script>")
		assert.NotContains(t, chunk.Content, "alert")
		assert.Contains(t, chunk.Content, "記事の本文テキスト")
	}
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
	// Old body: single merged chunk (short paragraphs merged)
	// New body: different content -> triggers update
	// Note: Chunker v8 merges short paragraphs (< 80 chars)
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
	// Old: single merged chunk
	mockChunkRepo.On("GetChunksByVersionID", ctx, verID).Return([]domain.RagChunk{
		{Ordinal: 0, Content: "Start.\n\nEnd.", ID: uuid.New()},
	}, nil)

	// 4. Create Version (v2)
	mockDocRepo.On("CreateVersion", ctx, mock.MatchedBy(func(v *domain.RagDocumentVersion) bool {
		return v.VersionNumber == 2
	})).Return(nil)

	// 5. Insert New Chunks (1 merged chunk)
	mockChunkRepo.On("BulkInsertChunks", ctx, mock.MatchedBy(func(chunks []domain.RagChunk) bool {
		return len(chunks) == 1 // Merged into 1 chunk due to MinChunkLength
	})).Return(nil)

	// 6. Insert Events
	// New content differs from old -> 1 "updated" event
	mockChunkRepo.On("InsertEvents", ctx, mock.MatchedBy(func(events []domain.RagChunkEvent) bool {
		return len(events) == 1
	})).Return(nil)

	// 7. Update Current Version
	mockDocRepo.On("UpdateCurrentVersion", ctx, docID, mock.Anything).Return(nil)

	err := uc.Upsert(ctx, articleID, title, "", body)
	assert.NoError(t, err)
	mockDocRepo.AssertExpectations(t)
	mockChunkRepo.AssertExpectations(t)
}
