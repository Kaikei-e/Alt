package gateway

import (
	"context"
	"errors"
	"search-indexer/domain"
	"search-indexer/driver"
	"testing"
	"time"
)

// Mock driver for testing
type mockSearchDriver struct {
	indexedDocs       []driver.SearchDocumentDriver
	searchResults     []driver.SearchDocumentDriver
	indexErr          error
	searchErr         error
	ensureErr         error
	synonymsErr       error
	pruneErr          error
	gotPruneOlderThan time.Duration

	// Recorded by SearchByUserIDWithDateFilter so tests can catch a
	// regression that silently drops the date window instead of forwarding
	// it to the driver.
	dateFilterCalls     int
	gotDateFilterQuery  string
	gotDateFilterUserID string
	gotPublishedAfter   *time.Time
	gotPublishedBefore  *time.Time
}

func (m *mockSearchDriver) IndexDocuments(ctx context.Context, docs []driver.SearchDocumentDriver) error {
	if m.indexErr != nil {
		return m.indexErr
	}
	m.indexedDocs = append(m.indexedDocs, docs...)
	return nil
}

func (m *mockSearchDriver) DeleteDocuments(ctx context.Context, ids []string) error {
	// Remove deleted documents from indexedDocs
	filtered := []driver.SearchDocumentDriver{}
	for _, doc := range m.indexedDocs {
		found := false
		for _, id := range ids {
			if doc.ID == id {
				found = true
				break
			}
		}
		if !found {
			filtered = append(filtered, doc)
		}
	}
	m.indexedDocs = filtered
	return nil
}

func (m *mockSearchDriver) EnsureIndex(ctx context.Context) error {
	if m.ensureErr != nil {
		return m.ensureErr
	}
	return nil
}

func (m *mockSearchDriver) SearchByUserIDWithDateFilter(ctx context.Context, query string, userID string, publishedAfter, publishedBefore *time.Time, limit int) ([]driver.SearchDocumentDriver, error) {
	m.dateFilterCalls++
	m.gotDateFilterQuery = query
	m.gotDateFilterUserID = userID
	m.gotPublishedAfter = publishedAfter
	m.gotPublishedBefore = publishedBefore
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchResults, nil
}

func (m *mockSearchDriver) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]driver.SearchDocumentDriver, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchResults, nil
}

func (m *mockSearchDriver) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]driver.SearchDocumentDriver, int64, error) {
	if m.searchErr != nil {
		return nil, 0, m.searchErr
	}
	return m.searchResults, int64(len(m.searchResults)), nil
}

func (m *mockSearchDriver) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	if m.synonymsErr != nil {
		return m.synonymsErr
	}
	return nil
}

func (m *mockSearchDriver) PruneTaskHistory(ctx context.Context, olderThan time.Duration) error {
	m.gotPruneOlderThan = olderThan
	if m.pruneErr != nil {
		return m.pruneErr
	}
	return nil
}

func TestSearchEngineGateway_IndexDocuments(t *testing.T) {
	now := time.Now()
	article, _ := domain.NewArticle("1", "Test Title", "Test Content", []string{"tag1", "tag2"}, now, "user1")
	domainDoc := domain.NewSearchDocument(article)

	tests := []struct {
		name        string
		docs        []domain.SearchDocument
		mockErr     error
		wantErr     bool
		validateDoc func(driver.SearchDocumentDriver) bool
	}{
		{
			name:    "successful indexing with domain to driver conversion",
			docs:    []domain.SearchDocument{domainDoc},
			mockErr: nil,
			wantErr: false,
			validateDoc: func(doc driver.SearchDocumentDriver) bool {
				return doc.ID == "1" &&
					doc.Title == "Test Title" &&
					doc.Content == "Test Content" &&
					len(doc.Tags) == 2 &&
					doc.Tags[0] == "tag1" &&
					doc.Tags[1] == "tag2"
			},
		},
		{
			name:    "driver indexing error",
			docs:    []domain.SearchDocument{domainDoc},
			mockErr: &driver.DriverError{Op: "IndexDocuments", Err: errors.New("index creation failed")},
			wantErr: true,
		},
		{
			name:    "empty documents",
			docs:    []domain.SearchDocument{},
			mockErr: nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := &mockSearchDriver{
				indexErr: tt.mockErr,
			}

			gateway := NewSearchEngineGateway(driver)

			err := gateway.IndexDocuments(context.Background(), tt.docs)

			if tt.wantErr {
				if err == nil {
					t.Errorf("IndexDocuments() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("IndexDocuments() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateDoc != nil && len(driver.indexedDocs) > 0 {
				if !tt.validateDoc(driver.indexedDocs[0]) {
					t.Errorf("Document conversion validation failed")
				}
			}

			if len(driver.indexedDocs) != len(tt.docs) {
				t.Errorf("IndexDocuments() indexed %d docs, want %d", len(driver.indexedDocs), len(tt.docs))
			}
		})
	}
}

func TestSearchEngineGateway_SearchByUserIDWithDateFilter(t *testing.T) {
	after := time.Unix(1700000000, 0)
	before := time.Unix(1700003600, 0)

	driverDoc := driver.SearchDocumentDriver{
		ID:      "1",
		Title:   "Test Title",
		Content: "Test Content",
		Tags:    []string{"tag1", "tag2"},
	}

	tests := []struct {
		name          string
		query         string
		userID        string
		after         *time.Time
		before        *time.Time
		limit         int
		mockResults   []driver.SearchDocumentDriver
		mockErr       error
		wantErr       bool
		wantCount     int
		validateFirst func(domain.SearchDocument) bool
	}{
		{
			name:        "successful search with driver to domain conversion",
			query:       "test",
			userID:      "u1",
			after:       &after,
			before:      &before,
			limit:       10,
			mockResults: []driver.SearchDocumentDriver{driverDoc},
			mockErr:     nil,
			wantErr:     false,
			wantCount:   1,
			validateFirst: func(doc domain.SearchDocument) bool {
				return doc.ID == "1" &&
					doc.Title == "Test Title" &&
					doc.Content == "Test Content" &&
					len(doc.Tags) == 2 &&
					doc.Tags[0] == "tag1" &&
					doc.Tags[1] == "tag2"
			},
		},
		{
			name:        "empty userID returns error",
			query:       "test",
			userID:      "",
			limit:       10,
			mockResults: nil,
			mockErr:     nil,
			wantErr:     true,
			wantCount:   0,
		},
		{
			name:        "driver search error",
			query:       "test",
			userID:      "u1",
			limit:       10,
			mockResults: nil,
			mockErr:     &driver.DriverError{Op: "SearchByUserIDWithDateFilter", Err: errors.New("search failed")},
			wantErr:     true,
			wantCount:   0,
		},
		{
			name:        "empty results",
			query:       "nonexistent",
			userID:      "u1",
			limit:       10,
			mockResults: []driver.SearchDocumentDriver{},
			mockErr:     nil,
			wantErr:     false,
			wantCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := &mockSearchDriver{
				searchResults: tt.mockResults,
				searchErr:     tt.mockErr,
			}

			gateway := NewSearchEngineGateway(driver)

			results, err := gateway.SearchByUserIDWithDateFilter(context.Background(), tt.query, tt.userID, tt.after, tt.before, tt.limit)

			if tt.wantErr {
				if err == nil {
					t.Errorf("SearchByUserIDWithDateFilter() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("SearchByUserIDWithDateFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(results) != tt.wantCount {
				t.Errorf("SearchByUserIDWithDateFilter() got %d results, want %d", len(results), tt.wantCount)
				return
			}

			if tt.validateFirst != nil && len(results) > 0 {
				if !tt.validateFirst(results[0]) {
					t.Errorf("First result validation failed")
				}
			}
		})
	}
}

// TestSearchEngineGateway_SearchByUserIDWithDateFilter_ForwardsBoundsToDriver
// guards against a regression that silently drops the date window: it
// asserts the driver actually received the publishedAfter/publishedBefore
// bounds the gateway was called with, not just that a result came back.
func TestSearchEngineGateway_SearchByUserIDWithDateFilter_ForwardsBoundsToDriver(t *testing.T) {
	after := time.Unix(1700000000, 0)
	before := time.Unix(1700003600, 0)

	mockDriver := &mockSearchDriver{
		searchResults: []driver.SearchDocumentDriver{{ID: "1"}},
	}
	gw := NewSearchEngineGateway(mockDriver)

	_, err := gw.SearchByUserIDWithDateFilter(context.Background(), "test", "u1", &after, &before, 10)
	if err != nil {
		t.Fatalf("SearchByUserIDWithDateFilter() unexpected error: %v", err)
	}

	if mockDriver.dateFilterCalls != 1 {
		t.Errorf("dateFilterCalls = %d, want 1", mockDriver.dateFilterCalls)
	}
	if mockDriver.gotDateFilterQuery != "test" || mockDriver.gotDateFilterUserID != "u1" {
		t.Errorf("driver received query=%q userID=%q, want query=%q userID=%q",
			mockDriver.gotDateFilterQuery, mockDriver.gotDateFilterUserID, "test", "u1")
	}
	if mockDriver.gotPublishedAfter == nil || !mockDriver.gotPublishedAfter.Equal(after) {
		t.Errorf("driver received publishedAfter = %v, want %v", mockDriver.gotPublishedAfter, after)
	}
	if mockDriver.gotPublishedBefore == nil || !mockDriver.gotPublishedBefore.Equal(before) {
		t.Errorf("driver received publishedBefore = %v, want %v", mockDriver.gotPublishedBefore, before)
	}
}

func TestSearchEngineGateway_EnsureIndex(t *testing.T) {
	tests := []struct {
		name    string
		mockErr error
		wantErr bool
	}{
		{
			name:    "successful index creation",
			mockErr: nil,
			wantErr: false,
		},
		{
			name:    "driver error",
			mockErr: &driver.DriverError{Op: "EnsureIndex", Err: errors.New("index creation failed")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := &mockSearchDriver{
				ensureErr: tt.mockErr,
			}

			gateway := NewSearchEngineGateway(driver)

			err := gateway.EnsureIndex(context.Background())

			if tt.wantErr && err == nil {
				t.Errorf("EnsureIndex() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && err != nil {
				t.Errorf("EnsureIndex() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSearchEngineGateway_PruneTaskHistory(t *testing.T) {
	tests := []struct {
		name    string
		mockErr error
		wantErr bool
	}{
		{
			name:    "successful prune",
			mockErr: nil,
			wantErr: false,
		},
		{
			name:    "driver error",
			mockErr: &driver.DriverError{Op: "PruneTaskHistory", Err: errors.New("meilisearch unreachable")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDriver := &mockSearchDriver{
				pruneErr: tt.mockErr,
			}

			gateway := NewSearchEngineGateway(mockDriver)

			err := gateway.PruneTaskHistory(context.Background(), 72*time.Hour)

			if tt.wantErr && err == nil {
				t.Errorf("PruneTaskHistory() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("PruneTaskHistory() error = %v, wantErr %v", err, tt.wantErr)
			}
			if mockDriver.gotPruneOlderThan != 72*time.Hour {
				t.Errorf("driver received olderThan = %v, want 72h", mockDriver.gotPruneOlderThan)
			}
		})
	}
}

func TestSearchEngineGateway_SearchByUserID_RejectsEmptyUserID(t *testing.T) {
	mockDriver := &mockSearchDriver{}
	gw := NewSearchEngineGateway(mockDriver)

	_, err := gw.SearchByUserID(context.Background(), "query", "", 10)
	if err == nil {
		t.Error("SearchByUserID with empty userID should return error")
	}

	_, err = gw.SearchByUserID(context.Background(), "query", "   ", 10)
	if err == nil {
		t.Error("SearchByUserID with whitespace userID should return error")
	}
}

func TestSearchEngineGateway_SearchByUserIDWithPagination_RejectsEmptyUserID(t *testing.T) {
	mockDriver := &mockSearchDriver{}
	gw := NewSearchEngineGateway(mockDriver)

	_, _, err := gw.SearchByUserIDWithPagination(context.Background(), "query", "", 0, 10)
	if err == nil {
		t.Error("SearchByUserIDWithPagination with empty userID should return error")
	}

	_, _, err = gw.SearchByUserIDWithPagination(context.Background(), "query", "   ", 0, 10)
	if err == nil {
		t.Error("SearchByUserIDWithPagination with whitespace userID should return error")
	}
}
