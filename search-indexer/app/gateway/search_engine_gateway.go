package gateway

import (
	"context"
	"search-indexer/domain"
	"search-indexer/driver"
	"search-indexer/port"
)

type SearchDriver interface {
	IndexDocuments(ctx context.Context, docs []driver.SearchDocumentDriver) error
	Search(ctx context.Context, query string, limit int) ([]driver.SearchDocumentDriver, error)
	EnsureIndex(ctx context.Context) error
}

type SearchEngineGateway struct {
	driver SearchDriver
}

func NewSearchEngineGateway(driver SearchDriver) *SearchEngineGateway {
	return &SearchEngineGateway{
		driver: driver,
	}
}

func (g *SearchEngineGateway) IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error {
	if len(docs) == 0 {
		return nil
	}

	driverDocs := make([]driver.SearchDocumentDriver, len(docs))
	for i, domainDoc := range docs {
		driverDocs[i] = driver.SearchDocumentDriver{
			ID:      domainDoc.ID,
			Title:   domainDoc.Title,
			Content: domainDoc.Content,
			Tags:    domainDoc.Tags,
		}
	}

	err := g.driver.IndexDocuments(ctx, driverDocs)
	if err != nil {
		return &port.SearchEngineError{
			Op:  "IndexDocuments",
			Err: err.Error(),
		}
	}

	return nil
}

func (g *SearchEngineGateway) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	driverResults, err := g.driver.Search(ctx, query, limit)
	if err != nil {
		return nil, &port.SearchEngineError{
			Op:  "Search",
			Err: err.Error(),
		}
	}

	domainResults := make([]domain.SearchDocument, len(driverResults))
	for i, driverDoc := range driverResults {
		domainResults[i] = domain.SearchDocument{
			ID:      driverDoc.ID,
			Title:   driverDoc.Title,
			Content: driverDoc.Content,
			Tags:    driverDoc.Tags,
		}
	}

	return domainResults, nil
}

func (g *SearchEngineGateway) EnsureIndex(ctx context.Context) error {
	err := g.driver.EnsureIndex(ctx)
	if err != nil {
		return &port.SearchEngineError{
			Op:  "EnsureIndex",
			Err: err.Error(),
		}
	}
	return nil
}
