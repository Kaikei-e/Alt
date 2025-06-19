package gateway

import (
	"context"
	"search-indexer/domain"
	"search-indexer/driver"
	"testing"
	"time"
)

// Mock driver for testing
type mockArticleDriver struct {
	articles []*driver.ArticleWithTags
	err      error
}

func (m *mockArticleDriver) GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*driver.ArticleWithTags, *time.Time, string, error) {
	if m.err != nil {
		return nil, nil, "", m.err
	}

	if len(m.articles) == 0 {
		return []*driver.ArticleWithTags{}, nil, "", nil
	}

	lastArticle := m.articles[len(m.articles)-1]
	return m.articles, &lastArticle.CreatedAt, lastArticle.ID, nil
}

func TestArticleRepositoryGateway_GetArticlesWithTags(t *testing.T) {
	now := time.Now()

	driverArticle1 := &driver.ArticleWithTags{
		ID:        "1",
		Title:     "Title 1",
		Content:   "Content 1",
		Tags:      []driver.TagModel{{Name: "tag1"}, {Name: "tag2"}},
		CreatedAt: now,
	}

	driverArticle2 := &driver.ArticleWithTags{
		ID:        "2",
		Title:     "Title 2",
		Content:   "Content 2",
		Tags:      []driver.TagModel{{Name: "tag3"}},
		CreatedAt: now.Add(time.Minute),
	}

	tests := []struct {
		name          string
		mockArticles  []*driver.ArticleWithTags
		mockErr       error
		lastCreatedAt *time.Time
		lastID        string
		limit         int
		wantCount     int
		wantErr       bool
		validateFirst func(*domain.Article) bool
	}{
		{
			name:         "successful conversion from driver to domain",
			mockArticles: []*driver.ArticleWithTags{driverArticle1, driverArticle2},
			mockErr:      nil,
			limit:        10,
			wantCount:    2,
			wantErr:      false,
			validateFirst: func(article *domain.Article) bool {
				return article.ID() == "1" &&
					article.Title() == "Title 1" &&
					len(article.Tags()) == 2 &&
					article.HasTag("tag1") &&
					article.HasTag("tag2")
			},
		},
		{
			name:         "empty result",
			mockArticles: []*driver.ArticleWithTags{},
			mockErr:      nil,
			limit:        10,
			wantCount:    0,
			wantErr:      false,
		},
		{
			name:         "driver error",
			mockArticles: nil,
			mockErr:      &DriverError{Op: "GetArticlesWithTags", Err: "database connection failed"},
			limit:        10,
			wantCount:    0,
			wantErr:      true,
		},
		{
			name: "invalid article data from driver",
			mockArticles: []*driver.ArticleWithTags{
				{
					ID:        "", // empty ID should cause domain validation error
					Title:     "Title",
					Content:   "Content",
					Tags:      []driver.TagModel{{Name: "tag1"}},
					CreatedAt: now,
				},
			},
			mockErr:   nil,
			limit:     10,
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := &mockArticleDriver{
				articles: tt.mockArticles,
				err:      tt.mockErr,
			}

			gateway := NewArticleRepositoryGateway(driver)

			articles, _, _, err := gateway.GetArticlesWithTags(context.Background(), tt.lastCreatedAt, tt.lastID, tt.limit)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetArticlesWithTags() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("GetArticlesWithTags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(articles) != tt.wantCount {
				t.Errorf("GetArticlesWithTags() got %d articles, want %d", len(articles), tt.wantCount)
				return
			}

			if tt.validateFirst != nil && len(articles) > 0 {
				if !tt.validateFirst(articles[0]) {
					t.Errorf("First article validation failed")
				}
			}
		})
	}
}

func TestArticleRepositoryGateway_ConvertToDomain(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		driverData  *driver.ArticleWithTags
		wantErr     bool
		validateRes func(*domain.Article) bool
	}{
		{
			name: "successful conversion",
			driverData: &driver.ArticleWithTags{
				ID:        "test-id",
				Title:     "Test Title",
				Content:   "Test Content",
				Tags:      []driver.TagModel{{Name: "tag1"}, {Name: "tag2"}},
				CreatedAt: now,
			},
			wantErr: false,
			validateRes: func(article *domain.Article) bool {
				return article.ID() == "test-id" &&
					article.Title() == "Test Title" &&
					article.Content() == "Test Content" &&
					len(article.Tags()) == 2 &&
					article.HasTag("tag1") &&
					article.HasTag("tag2")
			},
		},
		{
			name: "empty ID should fail",
			driverData: &driver.ArticleWithTags{
				ID:        "",
				Title:     "Test Title",
				Content:   "Test Content",
				Tags:      []driver.TagModel{{Name: "tag1"}},
				CreatedAt: now,
			},
			wantErr: true,
		},
		{
			name: "empty title should fail",
			driverData: &driver.ArticleWithTags{
				ID:        "test-id",
				Title:     "",
				Content:   "Test Content",
				Tags:      []driver.TagModel{{Name: "tag1"}},
				CreatedAt: now,
			},
			wantErr: true,
		},
	}

	gateway := &ArticleRepositoryGateway{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			article, err := gateway.convertToDomain(tt.driverData)

			if tt.wantErr {
				if err == nil {
					t.Errorf("convertToDomain() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("convertToDomain() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.validateRes != nil && !tt.validateRes(article) {
				t.Errorf("convertToDomain() validation failed")
			}
		})
	}
}
