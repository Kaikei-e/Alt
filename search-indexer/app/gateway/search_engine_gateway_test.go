package gateway

import (
	"context"
	"search-indexer/domain"
	"search-indexer/driver"
	"testing"
	"time"
)

// Mock driver for testing
type mockSearchDriver struct {
	indexedDocs   []driver.SearchDocumentDriver
	searchResults []driver.SearchDocumentDriver
	indexErr      error
	searchErr     error
	ensureErr     error
}

func (m *mockSearchDriver) IndexDocuments(ctx context.Context, docs []driver.SearchDocumentDriver) error {
	if m.indexErr != nil {
		return m.indexErr
	}
	m.indexedDocs = append(m.indexedDocs, docs...)
	return nil
}

func (m *mockSearchDriver) Search(ctx context.Context, query string, limit int) ([]driver.SearchDocumentDriver, error) {
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	return m.searchResults, nil
}

func (m *mockSearchDriver) EnsureIndex(ctx context.Context) error {
	if m.ensureErr != nil {
		return m.ensureErr
	}
	return nil
}

func TestSearchEngineGateway_IndexDocuments(t *testing.T) {
	now := time.Now()
	article, _ := domain.NewArticle("1", "Test Title", "Test Content", []string{"tag1", "tag2"}, now)
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
			mockErr: &driver.DriverError{Op: "IndexDocuments", Err: "index creation failed"},
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

func TestSearchEngineGateway_Search(t *testing.T) {

	driverDoc := driver.SearchDocumentDriver{
		ID:      "1",
		Title:   "Test Title",
		Content: "Test Content",
		Tags:    []string{"tag1", "tag2"},
	}

	tests := []struct {
		name          string
		query         string
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
			name:        "driver search error",
			query:       "test",
			limit:       10,
			mockResults: nil,
			mockErr:     &driver.DriverError{Op: "Search", Err: "search failed"},
			wantErr:     true,
			wantCount:   0,
		},
		{
			name:        "empty results",
			query:       "nonexistent",
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

			results, err := gateway.Search(context.Background(), tt.query, tt.limit)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Search() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("Search() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(results) != tt.wantCount {
				t.Errorf("Search() got %d results, want %d", len(results), tt.wantCount)
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
			mockErr: &driver.DriverError{Op: "EnsureIndex", Err: "index creation failed"},
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
