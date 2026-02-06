package gateway

import (
	"context"
	"search-indexer/domain"
	"search-indexer/driver"
)

type SearchDriver interface {
	IndexDocuments(ctx context.Context, docs []driver.SearchDocumentDriver) error
	DeleteDocuments(ctx context.Context, ids []string) error
	Search(ctx context.Context, query string, limit int) ([]driver.SearchDocumentDriver, error)
	SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]driver.SearchDocumentDriver, error)
	SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]driver.SearchDocumentDriver, error)
	SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]driver.SearchDocumentDriver, int64, error)
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
			UserID:  domainDoc.UserID,
		}
	}

	err := g.driver.IndexDocuments(ctx, driverDocs)
	if err != nil {
		return &domain.SearchEngineError{
			Op:  "IndexDocuments",
			Err: err.Error(),
		}
	}

	return nil
}

func (g *SearchEngineGateway) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	err := g.driver.DeleteDocuments(ctx, ids)
	if err != nil {
		return &domain.SearchEngineError{
			Op:  "DeleteDocuments",
			Err: err.Error(),
		}
	}

	return nil
}

func (g *SearchEngineGateway) Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error) {
	driverResults, err := g.driver.Search(ctx, query, limit)
	if err != nil {
		return nil, &domain.SearchEngineError{
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
			UserID:  driverDoc.UserID,
		}
	}

	return domainResults, nil
}

func (g *SearchEngineGateway) SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error) {
	driverResults, err := g.driver.SearchWithFilters(ctx, query, filters, limit)
	if err != nil {
		return nil, &domain.SearchEngineError{
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
			UserID:  driverDoc.UserID,
		}
	}

	return domainResults, nil
}

func (g *SearchEngineGateway) SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error) {
	driverResults, err := g.driver.SearchByUserID(ctx, query, userID, limit)
	if err != nil {
		return nil, &domain.SearchEngineError{Op: "SearchByUserID", Err: err.Error()}
	}
	return g.convertDocs(driverResults), nil
}

func (g *SearchEngineGateway) SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error) {
	driverResults, total, err := g.driver.SearchByUserIDWithPagination(ctx, query, userID, offset, limit)
	if err != nil {
		return nil, 0, &domain.SearchEngineError{Op: "SearchByUserIDWithPagination", Err: err.Error()}
	}
	return g.convertDocs(driverResults), total, nil
}

func (g *SearchEngineGateway) convertDocs(driverResults []driver.SearchDocumentDriver) []domain.SearchDocument {
	domainResults := make([]domain.SearchDocument, len(driverResults))
	for i, d := range driverResults {
		domainResults[i] = domain.SearchDocument{
			ID:      d.ID,
			Title:   d.Title,
			Content: d.Content,
			Tags:    d.Tags,
			UserID:  d.UserID,
		}
	}
	return domainResults
}

func (g *SearchEngineGateway) EnsureIndex(ctx context.Context) error {
	err := g.driver.EnsureIndex(ctx)
	if err != nil {
		return &domain.SearchEngineError{
			Op:  "EnsureIndex",
			Err: err.Error(),
		}
	}
	return nil
}

func (g *SearchEngineGateway) RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error {
	err := g.driver.RegisterSynonyms(ctx, synonyms)
	if err != nil {
		return &domain.SearchEngineError{
			Op:  "RegisterSynonyms",
			Err: err.Error(),
		}
	}
	return nil
}
