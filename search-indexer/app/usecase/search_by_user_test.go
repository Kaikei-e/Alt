package usecase

import (
	"context"
	"search-indexer/domain"
	"strings"
	"testing"
	"time"
)

func TestSearchByUserUsecase_Execute_RequiresUserID(t *testing.T) {
	mock := &mockSearchEngine{}
	uc := NewSearchByUserUsecase(mock)

	tests := []struct {
		name   string
		userID string
	}{
		{"empty user_id", ""},
		{"whitespace user_id", "   "},
		{"tab user_id", "\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uc.Execute(context.Background(), "test query", tt.userID)
			if err == nil {
				t.Errorf("Execute() with userID=%q should fail", tt.userID)
			}
			if !strings.Contains(err.Error(), "user_id parameter required") {
				t.Errorf("Execute() error = %v, want containing 'user_id parameter required'", err)
			}
		})
	}
}

func TestSearchByUserUsecase_ExecuteWithPagination_RequiresUserID(t *testing.T) {
	mock := &mockSearchEngine{}
	uc := NewSearchByUserUsecase(mock)

	tests := []struct {
		name   string
		userID string
	}{
		{"empty user_id", ""},
		{"whitespace user_id", "   "},
		{"tab user_id", "\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uc.ExecuteWithPagination(context.Background(), "test query", tt.userID, 0, 10)
			if err == nil {
				t.Errorf("ExecuteWithPagination() with userID=%q should fail", tt.userID)
			}
			if !strings.Contains(err.Error(), "user_id is required") {
				t.Errorf("ExecuteWithPagination() error = %v, want containing 'user_id is required'", err)
			}
		})
	}
}

func TestSearchByUserUsecase_Execute_Success(t *testing.T) {
	article, _ := domain.NewArticle("1", "Title", "Content", []string{"tag1"}, time.Now(), "user1")
	doc := domain.NewSearchDocument(article)
	mock := &mockSearchEngine{
		indexedDocs: []domain.SearchDocument{doc},
	}
	uc := NewSearchByUserUsecase(mock)

	result, err := uc.Execute(context.Background(), "test query", "user1")
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Errorf("Hits count = %d, want 1", len(result.Hits))
	}
}

func TestSearchByUserUsecase_ExecuteWithPagination_Success(t *testing.T) {
	article, _ := domain.NewArticle("1", "Title", "Content", []string{"tag1"}, time.Now(), "user1")
	doc := domain.NewSearchDocument(article)
	mock := &mockSearchEngine{
		indexedDocs: []domain.SearchDocument{doc},
	}
	uc := NewSearchByUserUsecase(mock)

	result, err := uc.ExecuteWithPagination(context.Background(), "test query", "user1", 0, 10)
	if err != nil {
		t.Fatalf("ExecuteWithPagination() unexpected error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Errorf("Hits count = %d, want 1", len(result.Hits))
	}
	if result.EstimatedTotalHits != 1 {
		t.Errorf("EstimatedTotalHits = %d, want 1", result.EstimatedTotalHits)
	}
}

func TestSearchByUserUsecase_ExecuteWithDateFilter_RequiresUserID(t *testing.T) {
	mock := &mockSearchEngine{}
	uc := NewSearchByUserUsecase(mock)
	now := time.Now()

	tests := []struct {
		name   string
		userID string
	}{
		{"empty user_id", ""},
		{"whitespace user_id", "   "},
		{"tab user_id", "\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uc.ExecuteWithDateFilter(context.Background(), "test query", tt.userID, &now, nil, 20)
			if err == nil {
				t.Errorf("ExecuteWithDateFilter() with userID=%q should fail", tt.userID)
			}
			if !strings.Contains(err.Error(), "user_id parameter required") {
				t.Errorf("ExecuteWithDateFilter() error = %v, want containing 'user_id parameter required'", err)
			}
		})
	}
}

func TestSearchByUserUsecase_ExecuteWithDateFilter_RejectsInvertedDates(t *testing.T) {
	mock := &mockSearchEngine{}
	uc := NewSearchByUserUsecase(mock)
	after := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)

	_, err := uc.ExecuteWithDateFilter(context.Background(), "test query", "user1", &after, &before, 20)
	if err == nil {
		t.Fatal("ExecuteWithDateFilter() with publishedAfter > publishedBefore should fail")
	}
	if !strings.Contains(err.Error(), "published_after must not be after published_before") {
		t.Errorf("ExecuteWithDateFilter() error = %v, want containing 'published_after must not be after published_before'", err)
	}
}

func TestSearchByUserUsecase_ExecuteWithDateFilter_Success(t *testing.T) {
	article, _ := domain.NewArticle("1", "Title", "Content", []string{"tag1"}, time.Now(), "user1")
	doc := domain.NewSearchDocument(article)
	mock := &mockSearchEngine{
		indexedDocs: []domain.SearchDocument{doc},
	}
	uc := NewSearchByUserUsecase(mock)
	after := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)

	result, err := uc.ExecuteWithDateFilter(context.Background(), "test query", "user1", &after, &before, 10)
	if err != nil {
		t.Fatalf("ExecuteWithDateFilter() unexpected error: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Errorf("Hits count = %d, want 1", len(result.Hits))
	}

	// Guard against a regression that drops the date window: the search
	// engine must actually receive the bounds ExecuteWithDateFilter was
	// called with, not just return a result.
	if mock.dateFilterCalls != 1 {
		t.Errorf("dateFilterCalls = %d, want 1", mock.dateFilterCalls)
	}
	if mock.gotDateFilterQuery != "test query" || mock.gotDateFilterUserID != "user1" {
		t.Errorf("search engine received query=%q userID=%q, want query=%q userID=%q",
			mock.gotDateFilterQuery, mock.gotDateFilterUserID, "test query", "user1")
	}
	if mock.gotPublishedAfter == nil || !mock.gotPublishedAfter.Equal(after) {
		t.Errorf("search engine received publishedAfter = %v, want %v", mock.gotPublishedAfter, after)
	}
	if mock.gotPublishedBefore == nil || !mock.gotPublishedBefore.Equal(before) {
		t.Errorf("search engine received publishedBefore = %v, want %v", mock.gotPublishedBefore, before)
	}
}
