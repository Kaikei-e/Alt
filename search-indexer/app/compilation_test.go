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

// TestCompilation is a cross-package smoke test verifying that domain,
// driver, gateway, and usecase types satisfy their expected contracts and
// construct correctly when wired together. It previously only logged
// results with t.Log (no assertions at all), so a real regression in any of
// these paths -- e.g. NewArticle silently failing validation, or a
// constructor returning nil -- would pass the suite unnoticed.
func TestCompilation(t *testing.T) {
	// Valid filter tags must pass validation.
	filters := []string{"test", "programming"}
	if err := domain.ValidateFilterTags(filters); err != nil {
		t.Errorf("ValidateFilterTags(%v) = %v, want nil", filters, err)
	}

	// A well-formed article must construct without error and expose the
	// fields it was given.
	article, err := domain.NewArticle("1", "Test Title", "Test Content", []string{"tag1"}, time.Now(), "user1")
	if err != nil {
		t.Fatalf("NewArticle() error = %v, want nil", err)
	}
	if article.ID() != "1" {
		t.Errorf("article.ID() = %q, want %q", article.ID(), "1")
	}
	if article.Title() != "Test Title" {
		t.Errorf("article.Title() = %q, want %q", article.Title(), "Test Title")
	}

	// The search document derived from an article must carry over its ID
	// and title unchanged.
	searchDoc := domain.NewSearchDocument(article)
	if searchDoc.ID != article.ID() {
		t.Errorf("searchDoc.ID = %q, want %q (article ID)", searchDoc.ID, article.ID())
	}
	if searchDoc.Title != "Test Title" {
		t.Errorf("searchDoc.Title = %q, want %q", searchDoc.Title, "Test Title")
	}

	// Driver-level document type: fields must round-trip through the struct
	// literal untouched.
	driverDoc := driver.SearchDocumentDriver{
		ID:      "1",
		Title:   "Test",
		Content: "Content",
		Tags:    []string{"tag1"},
	}
	if driverDoc.ID != "1" || driverDoc.Title != "Test" || driverDoc.Content != "Content" {
		t.Errorf("driverDoc = %+v, want ID=1 Title=Test Content=Content", driverDoc)
	}

	// Compile-time interface satisfaction: SearchEngineGateway must implement
	// port.SearchEngine. This line fails to compile (not just fails a test)
	// if the gateway drifts from the port contract.
	var _ port.SearchEngine = (*gateway.SearchEngineGateway)(nil)

	// Usecase constructors must return usable, non-nil instances even when
	// wired with a nil dependency (the real dependency is injected later by
	// bootstrap; the constructor itself must not panic or return nil).
	searchUsecase := usecase.NewSearchArticlesUsecase(nil)
	if searchUsecase == nil {
		t.Fatal("NewSearchArticlesUsecase(nil) = nil, want non-nil usecase")
	}

	searchByUserUsecase := usecase.NewSearchByUserUsecase(nil)
	if searchByUserUsecase == nil {
		t.Fatal("NewSearchByUserUsecase(nil) = nil, want non-nil usecase")
	}
}
