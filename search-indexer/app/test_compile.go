package main

import (
	"context"
	"fmt"
	"search-indexer/domain"
	"search-indexer/driver"
	"search-indexer/gateway"
	"search-indexer/port"
	"search-indexer/search_engine"
	"search-indexer/usecase"
	"time"
)

func main() {
	// Test basic compilation by creating instances
	ctx := context.Background()
	
	// Test search_engine functions
	filters := []string{"test", "programming"}
	filter := search_engine.MakeSecureSearchFilter(filters)
	fmt.Println("Filter:", filter)
	
	err := search_engine.ValidateFilterTags(filters)
	if err != nil {
		fmt.Println("Validation error:", err)
	}
	
	// Test domain objects
	article, err := domain.NewArticle("1", "Test Title", "Test Content", []string{"tag1"}, time.Now())
	if err != nil {
		fmt.Println("Article creation error:", err)
	}
	
	searchDoc := domain.NewSearchDocument(article)
	fmt.Println("Document ID:", searchDoc.ID)
	
	// Test driver types
	driverDoc := driver.SearchDocumentDriver{
		ID:      "1",
		Title:   "Test",
		Content: "Content",
		Tags:    []string{"tag1"},
	}
	fmt.Println("Driver document:", driverDoc.ID)
	
	// Test port interfaces exist
	var _ port.SearchEngine = (*gateway.SearchEngineGateway)(nil)
	
	// Test usecase compilation
	searchUsecase := usecase.NewSearchArticlesUsecase(nil)
	fmt.Println("Search usecase created:", searchUsecase != nil)
	
	searchWithFiltersUsecase := usecase.NewSearchArticlesWithFiltersUsecase(nil)
	fmt.Println("Search with filters usecase created:", searchWithFiltersUsecase != nil)
	
	fmt.Println("All types compile successfully!")
}