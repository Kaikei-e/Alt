package main

import (
	"search-indexer/domain"
	"search-indexer/driver"
	"search-indexer/gateway"
	"search-indexer/port"
	"search-indexer/search_engine"
	"search-indexer/usecase"
	"testing"
	"time"
)

func TestCompilation(t *testing.T) {
	// Test basic compilation by creating instances

	// Test search_engine functions
	filters := []string{"test", "programming"}
	filter := search_engine.MakeSecureSearchFilter(filters)
	t.Log("Filter:", filter)

	err := search_engine.ValidateFilterTags(filters)
	if err != nil {
		t.Log("Validation error:", err)
	}

	// Test domain objects
	article, err := domain.NewArticle("1", "Test Title", "Test Content", []string{"tag1"}, time.Now())
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

	t.Log("All types compile successfully!")
}
