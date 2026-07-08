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

func (m *mockArticleDriver) GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*driver.ArticleWithTags, *time.Time, string, error) {
	if m.err != nil {
		return nil, nil, "", m.err
	}

	if len(m.articles) == 0 {
		return []*driver.ArticleWithTags{}, nil, "", nil
	}

	lastArticle := m.articles[len(m.articles)-1]
	return m.articles, &lastArticle.CreatedAt, lastArticle.ID, nil
}

func (m *mockArticleDriver) GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]*driver.DeletedArticle, *time.Time, error) {
	if m.err != nil {
		return nil, nil, m.err
	}

	return []*driver.DeletedArticle{}, nil, nil
}

func (m *mockArticleDriver) GetLatestCreatedAt(ctx context.Context) (*time.Time, error) {
	if m.err != nil {
		return nil, m.err
	}

	if len(m.articles) == 0 {
		return nil, nil
	}

	latest := m.articles[0].CreatedAt
	for _, article := range m.articles {
		if article.CreatedAt.After(latest) {
			latest = article.CreatedAt
		}
	}

	return &latest, nil
}

func (m *mockArticleDriver) GetArticleByID(ctx context.Context, articleID string) (*driver.ArticleWithTags, error) {
	if m.err != nil {
		return nil, m.err
	}

	for _, article := range m.articles {
		if article.ID == articleID {
			return article, nil
		}
	}

	return nil, nil
}

func TestArticleRepositoryGateway_GetArticlesWithTags(t *testing.T) {
	now := time.Now()

	driverArticle1 := &driver.ArticleWithTags{
		ID:        "1",
		Title:     "Title 1",
		Content:   "Content 1",
		Tags:      []driver.TagModel{{TagName: "tag1"}, {TagName: "tag2"}},
		CreatedAt: now,
	}

	driverArticle2 := &driver.ArticleWithTags{
		ID:        "2",
		Title:     "Title 2",
		Content:   "Content 2",
		Tags:      []driver.TagModel{{TagName: "tag3"}},
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
			mockErr:      &driver.DriverError{Op: "GetArticlesWithTags", Err: errors.New("database connection failed")},
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
					Tags:      []driver.TagModel{{TagName: "tag1"}},
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
				Tags:      []driver.TagModel{{TagName: "tag1"}, {TagName: "tag2"}},
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
				Tags:      []driver.TagModel{{TagName: "tag1"}},
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
				Tags:      []driver.TagModel{{TagName: "tag1"}},
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

// TestArticleRepositoryGateway_GetArticleByID_NotFound pins the not-found
// contract: the driver signals "no such article" with (nil, nil) — see
// mockArticleDriver.GetArticleByID above, which mirrors backend_api's
// Client.GetArticleByID when alt-backend returns an empty Article. Before
// this fix the gateway wrapped that into a generic *domain.RepositoryError,
// which usecase.ExecuteBatchArticles could not distinguish from a real
// failure, so a single deleted article ID failed the entire batch and lost
// every other (already ACKed) article in it.
func TestArticleRepositoryGateway_GetArticleByID_NotFound(t *testing.T) {
	driverMock := &mockArticleDriver{}
	gw := NewArticleRepositoryGateway(driverMock)

	article, err := gw.GetArticleByID(context.Background(), "missing-id")

	if article != nil {
		t.Errorf("GetArticleByID() article = %v, want nil", article)
	}
	if !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("GetArticleByID() error = %v, want errors.Is(err, domain.ErrArticleNotFound)", err)
	}
}

// TestArticleRepositoryGateway_GetArticleByID_DriverError confirms a genuine
// driver failure (as opposed to not-found) still surfaces as a
// *domain.RepositoryError rather than being swallowed by the not-found path.
func TestArticleRepositoryGateway_GetArticleByID_DriverError(t *testing.T) {
	driverMock := &mockArticleDriver{err: errors.New("connection refused")}
	gw := NewArticleRepositoryGateway(driverMock)

	article, err := gw.GetArticleByID(context.Background(), "any-id")

	if article != nil {
		t.Errorf("GetArticleByID() article = %v, want nil", article)
	}
	if errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("GetArticleByID() error should not be ErrArticleNotFound for a driver failure, got %v", err)
	}
	var repoErr *domain.RepositoryError
	if !errors.As(err, &repoErr) {
		t.Fatalf("GetArticleByID() error = %v, want *domain.RepositoryError", err)
	}
}
