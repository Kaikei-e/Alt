package gateway

import (
	"context"
	"search-indexer/domain"
	"search-indexer/driver"
)

// RecapSearchDriver defines the driver interface for recap Meilisearch operations.
type RecapSearchDriver interface {
	EnsureIndex(ctx context.Context) error
	IndexDocuments(ctx context.Context, docs []driver.RecapDocumentDriver) error
	Search(ctx context.Context, query string, limit int) ([]driver.RecapDocumentDriver, int64, error)
}

// RecapSearchEngineGateway converts between domain and driver recap documents.
type RecapSearchEngineGateway struct {
	driver RecapSearchDriver
}

// NewRecapSearchEngineGateway creates a new gateway.
func NewRecapSearchEngineGateway(driver RecapSearchDriver) *RecapSearchEngineGateway {
	return &RecapSearchEngineGateway{driver: driver}
}

// EnsureRecapIndex ensures the recaps index exists and is configured.
func (g *RecapSearchEngineGateway) EnsureRecapIndex(ctx context.Context) error {
	if err := g.driver.EnsureIndex(ctx); err != nil {
		return &domain.SearchEngineError{Op: "EnsureRecapIndex", Err: err.Error()}
	}
	return nil
}

// IndexRecapDocuments indexes recap documents into Meilisearch.
func (g *RecapSearchEngineGateway) IndexRecapDocuments(ctx context.Context, docs []domain.RecapDocument) error {
	if len(docs) == 0 {
		return nil
	}

	driverDocs := make([]driver.RecapDocumentDriver, len(docs))
	for i, d := range docs {
		driverDocs[i] = driver.RecapDocumentDriver{
			ID:         d.ID,
			JobID:      d.JobID,
			ExecutedAt: d.ExecutedAt,
			WindowDays: d.WindowDays,
			Genre:      d.Genre,
			Summary:    d.Summary,
			TopTerms:   d.TopTerms,
			Tags:       d.Tags,
			Bullets:    d.Bullets,
		}
	}

	if err := g.driver.IndexDocuments(ctx, driverDocs); err != nil {
		return &domain.SearchEngineError{Op: "IndexRecapDocuments", Err: err.Error()}
	}
	return nil
}

// SearchRecaps searches the recaps index.
func (g *RecapSearchEngineGateway) SearchRecaps(ctx context.Context, query string, limit int) ([]domain.RecapDocument, int64, error) {
	driverDocs, total, err := g.driver.Search(ctx, query, limit)
	if err != nil {
		return nil, 0, &domain.SearchEngineError{Op: "SearchRecaps", Err: err.Error()}
	}

	docs := make([]domain.RecapDocument, len(driverDocs))
	for i, d := range driverDocs {
		docs[i] = domain.RecapDocument{
			ID:         d.ID,
			JobID:      d.JobID,
			ExecutedAt: d.ExecutedAt,
			WindowDays: d.WindowDays,
			Genre:      d.Genre,
			Summary:    d.Summary,
			TopTerms:   d.TopTerms,
			Tags:       d.Tags,
			Bullets:    d.Bullets,
		}
	}

	return docs, total, nil
}
