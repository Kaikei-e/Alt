package gateway

import (
	"context"
	"search-indexer/domain"
	"search-indexer/driver"
	"search-indexer/port"
	"time"
)

type ArticleDriver interface {
	GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*driver.ArticleWithTags, *time.Time, string, error)
	GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*driver.ArticleWithTags, *time.Time, string, error)
	GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]*driver.DeletedArticle, *time.Time, error)
	GetLatestCreatedAt(ctx context.Context) (*time.Time, error)
}

type ArticleRepositoryGateway struct {
	driver ArticleDriver
}

func NewArticleRepositoryGateway(driver ArticleDriver) *ArticleRepositoryGateway {
	return &ArticleRepositoryGateway{
		driver: driver,
	}
}

func (g *ArticleRepositoryGateway) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	driverArticles, newLastCreatedAt, newLastID, err := g.driver.GetArticlesWithTags(ctx, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", &port.RepositoryError{
			Op:  "GetArticlesWithTags",
			Err: err.Error(),
		}
	}

	if len(driverArticles) == 0 {
		return []*domain.Article{}, newLastCreatedAt, newLastID, nil
	}

	domainArticles := make([]*domain.Article, 0, len(driverArticles))
	for _, driverArticle := range driverArticles {
		domainArticle, err := g.convertToDomain(driverArticle)
		if err != nil {
			return nil, nil, "", &port.RepositoryError{
				Op:  "GetArticlesWithTags",
				Err: "failed to convert article to domain: id=" + driverArticle.ID + ", " + err.Error(),
			}
		}
		domainArticles = append(domainArticles, domainArticle)
	}

	return domainArticles, newLastCreatedAt, newLastID, nil
}

func (g *ArticleRepositoryGateway) convertToDomain(driverArticle *driver.ArticleWithTags) (*domain.Article, error) {
	tags := make([]string, len(driverArticle.Tags))
	for i, tag := range driverArticle.Tags {
		tags[i] = tag.TagName
	}

	return domain.NewArticle(
		driverArticle.ID,
		driverArticle.Title,
		driverArticle.Content,
		tags,
		driverArticle.CreatedAt,
		driverArticle.UserID,
	)
}

func (g *ArticleRepositoryGateway) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error) {
	driverArticles, newLastCreatedAt, newLastID, err := g.driver.GetArticlesWithTagsForward(ctx, incrementalMark, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", &port.RepositoryError{
			Op:  "GetArticlesWithTagsForward",
			Err: err.Error(),
		}
	}

	if len(driverArticles) == 0 {
		return []*domain.Article{}, newLastCreatedAt, newLastID, nil
	}

	domainArticles := make([]*domain.Article, 0, len(driverArticles))
	for _, driverArticle := range driverArticles {
		domainArticle, err := g.convertToDomain(driverArticle)
		if err != nil {
			return nil, nil, "", &port.RepositoryError{
				Op:  "GetArticlesWithTagsForward",
				Err: "failed to convert article to domain: id=" + driverArticle.ID + ", " + err.Error(),
			}
		}
		domainArticles = append(domainArticles, domainArticle)
	}

	return domainArticles, newLastCreatedAt, newLastID, nil
}

func (g *ArticleRepositoryGateway) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]string, *time.Time, error) {
	driverDeletedArticles, newLastDeletedAt, err := g.driver.GetDeletedArticles(ctx, lastDeletedAt, limit)
	if err != nil {
		return nil, nil, &port.RepositoryError{
			Op:  "GetDeletedArticles",
			Err: err.Error(),
		}
	}

	if len(driverDeletedArticles) == 0 {
		return []string{}, newLastDeletedAt, nil
	}

	ids := make([]string, len(driverDeletedArticles))
	for i, deletedArticle := range driverDeletedArticles {
		ids[i] = deletedArticle.ID
	}

	return ids, newLastDeletedAt, nil
}

func (g *ArticleRepositoryGateway) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	latestCreatedAt, err := g.driver.GetLatestCreatedAt(ctx)
	if err != nil {
		return nil, &port.RepositoryError{
			Op:  "GetLatestCreatedAt",
			Err: err.Error(),
		}
	}

	return latestCreatedAt, nil
}
