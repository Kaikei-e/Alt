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
	)
}
