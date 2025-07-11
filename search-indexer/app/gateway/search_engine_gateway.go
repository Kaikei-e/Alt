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
	SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]driver.SearchDocumentDriver, error)
	EnsureIndex(ctx context.Context) error
	RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error
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

func (g *SearchEngineGateway) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
	driverResults, err := g.driver.SearchWithFilters(ctx, query, filters, limit)
	if err != nil {
		return nil, &port.SearchEngineError{
			Op:  "SearchWithFilters",
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

func (g *SearchEngineGateway) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	err := g.driver.RegisterSynonyms(ctx, synonyms)
	if err != nil {
		return &port.SearchEngineError{
			Op:  "RegisterSynonyms",
			Err: err.Error(),
		}
	}
	return nil
}
