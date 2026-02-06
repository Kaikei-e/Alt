package main

import (
	"search-indexer/domain"
	"search-indexer/driver"
	"search-indexer/gateway"
	"search-indexer/port"
	"search-indexer/usecase"
	"testing"
	"time"
)

func TestCompilation(t *testing.T) {
	// Test domain filter validation
	filters := []string{"test", "programming"}
	err := domain.ValidateFilterTags(filters)
	if err != nil {
		t.Log("Validation error:", err)
	}

	// Test domain objects
	article, err := domain.NewArticle("1", "Test Title", "Test Content", []string{"tag1"}, time.Now(), "user1")
	if err != nil {
		t.Log("Article creation error:", err)
	}

	searchDoc := domain.NewSearchDocument(article)
	t.Log("Document ID:", searchDoc.ID)

	// Test driver types
	driverDoc := driver.SearchDocumentDriver{
		ID:      "1",
		Title:   "Test",
		Content: "Content",
		Tags:    []string{"tag1"},
	}
	t.Log("Driver document:", driverDoc.ID)

	// Test port interfaces exist
	var _ port.SearchEngine = (*gateway.SearchEngineGateway)(nil)

	// Test usecase compilation
	searchUsecase := usecase.NewSearchArticlesUsecase(nil)
	t.Log("Search usecase created:", searchUsecase != nil)

	searchWithFiltersUsecase := usecase.NewSearchArticlesWithFiltersUsecase(nil)
	t.Log("Search with filters usecase created:", searchWithFiltersUsecase != nil)

	searchByUserUsecase := usecase.NewSearchByUserUsecase(nil)
	t.Log("Search by user usecase created:", searchByUserUsecase != nil)

	t.Log("All types compile successfully!")
}
