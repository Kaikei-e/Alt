package internal_article_gateway

import (
	"reflect"
	"testing"
	"time"

	"alt/dataplane/port/internal_article_port"
	"alt/dataplane/port/internal_feed_port"
	"alt/dataplane/port/internal_tag_port"
	"alt/shared/driver/alt_db"
)

func TestToPortArticle(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   *alt_db.InternalArticleWithTags
		want *internal_article_port.ArticleWithTags
	}{
		{
			name: "valid article with tags",
			in: &alt_db.InternalArticleWithTags{
				ID:          "art-1",
				Title:       "Title",
				Content:     "Content",
				Tags:        []string{"go", "ai"},
				CreatedAt:   now,
				UserID:      "user-1",
				Language:    "en",
				PublishedAt: &now,
			},
			want: &internal_article_port.ArticleWithTags{
				ID:          "art-1",
				Title:       "Title",
				Content:     "Content",
				Tags:        []string{"go", "ai"},
				CreatedAt:   now,
				UserID:      "user-1",
				Language:    "en",
				PublishedAt: &now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toPortArticle(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toPortArticle() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestToPortArticles(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   []*alt_db.InternalArticleWithTags
		want []*internal_article_port.ArticleWithTags
	}{
		{
			name: "empty slice",
			in:   []*alt_db.InternalArticleWithTags{},
			want: []*internal_article_port.ArticleWithTags{},
		},
		{
			name: "multiple items",
			in: []*alt_db.InternalArticleWithTags{
				{
					ID:        "art-1",
					Title:     "Article 1",
					CreatedAt: now,
				},
				{
					ID:        "art-2",
					Title:     "Article 2",
					CreatedAt: now.Add(time.Hour),
				},
			},
			want: []*internal_article_port.ArticleWithTags{
				{
					ID:        "art-1",
					Title:     "Article 1",
					CreatedAt: now,
				},
				{
					ID:        "art-2",
					Title:     "Article 2",
					CreatedAt: now.Add(time.Hour),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toPortArticles(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toPortArticles() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestToPortDeletedArticles(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   []*alt_db.InternalDeletedArticle
		want []*internal_article_port.DeletedArticle
	}{
		{
			name: "empty slice",
			in:   []*alt_db.InternalDeletedArticle{},
			want: []*internal_article_port.DeletedArticle{},
		},
		{
			name: "single item",
			in: []*alt_db.InternalDeletedArticle{
				{
					ID:        "del-1",
					DeletedAt: now,
				},
			},
			want: []*internal_article_port.DeletedArticle{
				{
					ID:        "del-1",
					DeletedAt: now,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toPortDeletedArticles(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toPortDeletedArticles() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestToDriverCreateArticleParams(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	in := internal_article_port.CreateArticleParams{
		Title:       "Sample",
		URL:         "https://example.com/sample",
		Content:     "Hello",
		FeedID:      "feed-1",
		UserID:      "user-1",
		Language:    "ja",
		PublishedAt: now,
	}
	want := alt_db.CreateArticleParams{
		Title:       "Sample",
		URL:         "https://example.com/sample",
		Content:     "Hello",
		FeedID:      "feed-1",
		UserID:      "user-1",
		Language:    "ja",
		PublishedAt: now,
	}

	got := toDriverCreateArticleParams(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toDriverCreateArticleParams() = %+v, want %+v", got, want)
	}
}

func TestToPortArticleContent(t *testing.T) {
	tests := []struct {
		name string
		in   *alt_db.InternalArticleContent
		want *internal_article_port.ArticleContent
	}{
		{
			name: "nil content",
			in:   nil,
			want: nil,
		},
		{
			name: "mapped content",
			in: &alt_db.InternalArticleContent{
				ID:      "id-1",
				Title:   "T",
				Content: "C",
				URL:     "U",
				UserID:  "UID",
			},
			want: &internal_article_port.ArticleContent{
				ID:      "id-1",
				Title:   "T",
				Content: "C",
				URL:     "U",
				UserID:  "UID",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toPortArticleContent(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toPortArticleContent() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestToPortFeedURLs(t *testing.T) {
	tests := []struct {
		name string
		in   []alt_db.InternalFeedURL
		want []internal_feed_port.FeedURL
	}{
		{
			name: "empty",
			in:   []alt_db.InternalFeedURL{},
			want: []internal_feed_port.FeedURL{},
		},
		{
			name: "multiple items",
			in: []alt_db.InternalFeedURL{
				{FeedID: "f-1", URL: "https://a.com"},
				{FeedID: "f-2", URL: "https://b.com"},
			},
			want: []internal_feed_port.FeedURL{
				{FeedID: "f-1", URL: "https://a.com"},
				{FeedID: "f-2", URL: "https://b.com"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toPortFeedURLs(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toPortFeedURLs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestToDriverTagUpsertItems(t *testing.T) {
	tests := []struct {
		name string
		in   []internal_tag_port.TagItem
		want []alt_db.TagUpsertItem
	}{
		{
			name: "empty",
			in:   []internal_tag_port.TagItem{},
			want: []alt_db.TagUpsertItem{},
		},
		{
			name: "items mapped",
			in: []internal_tag_port.TagItem{
				{Name: "tag1", Confidence: 0.95},
				{Name: "tag2", Confidence: 0.80},
			},
			want: []alt_db.TagUpsertItem{
				{Name: "tag1", Confidence: 0.95},
				{Name: "tag2", Confidence: 0.80},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toDriverTagUpsertItems(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toDriverTagUpsertItems() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestToDriverBatchUpsertTagItems(t *testing.T) {
	in := []internal_tag_port.BatchUpsertItem{
		{
			ArticleID: "art-1",
			FeedID:    "feed-1",
			Tags: []internal_tag_port.TagItem{
				{Name: "tech", Confidence: 0.9},
			},
		},
	}
	want := []alt_db.BatchUpsertTagItem{
		{
			ArticleID: "art-1",
			FeedID:    "feed-1",
			Tags: []alt_db.TagUpsertItem{
				{Name: "tech", Confidence: 0.9},
			},
		},
	}

	got := toDriverBatchUpsertTagItems(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toDriverBatchUpsertTagItems() = %+v, want %+v", got, want)
	}
}

func TestGroupTagsByArticleIDs(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	rows := []alt_db.BatchArticleTagRow{
		{ArticleID: "art-1", TagName: "golang", Confidence: 0.9, UpdatedAt: now},
		{ArticleID: "art-1", TagName: "cloud", Confidence: 0.8, UpdatedAt: now},
		{ArticleID: "art-2", TagName: "rust", Confidence: 0.95, UpdatedAt: now},
	}
	articleIDs := []string{"art-1", "art-2"}

	want := []internal_tag_port.ArticleTagsByID{
		{
			ArticleID: "art-1",
			Tags: []internal_tag_port.ArticleTagEntry{
				{TagName: "golang", Confidence: 0.9, UpdatedAt: now},
				{TagName: "cloud", Confidence: 0.8, UpdatedAt: now},
			},
		},
		{
			ArticleID: "art-2",
			Tags: []internal_tag_port.ArticleTagEntry{
				{TagName: "rust", Confidence: 0.95, UpdatedAt: now},
			},
		},
	}

	got := groupTagsByArticleIDs(rows, articleIDs)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("groupTagsByArticleIDs() = %+v, want %+v", got, want)
	}
}

func TestToPortUntaggedArticles(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	feedID := "F"
	in := []alt_db.InternalUntaggedArticle{
		{
			ID:        "art-1",
			Title:     "T",
			Content:   "C",
			UserID:    "U",
			FeedID:    &feedID,
			CreatedAt: now,
		},
	}
	want := []internal_tag_port.UntaggedArticle{
		{
			ID:        "art-1",
			Title:     "T",
			Content:   "C",
			UserID:    "U",
			FeedID:    &feedID,
			CreatedAt: now,
		},
	}

	got := toPortUntaggedArticles(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toPortUntaggedArticles() = %+v, want %+v", got, want)
	}
}

func TestToPortArticlesWithSummaryResults(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	in := []alt_db.ArticleWithSummaryResult{
		{
			ArticleID:       "art-1",
			ArticleContent:  "Content",
			ArticleURL:      "https://example.com/1",
			SummaryID:       "sum-1",
			SummaryJapanese: "日本語サマリー",
			CreatedAt:       now,
		},
	}
	want := []*internal_article_port.ArticleWithSummaryResult{
		{
			ArticleID:       "art-1",
			ArticleContent:  "Content",
			ArticleURL:      "https://example.com/1",
			SummaryID:       "sum-1",
			SummaryJapanese: "日本語サマリー",
			CreatedAt:       now,
		},
	}

	got := toPortArticlesWithSummaryResults(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toPortArticlesWithSummaryResults() = %+v, want %+v", got, want)
	}
}

func TestToPortUnsummarizedArticles(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	in := []alt_db.InternalUnsummarizedArticle{
		{
			ID:        "art-1",
			Title:     "T",
			Content:   "C",
			URL:       "https://example.com/1",
			CreatedAt: now,
			UserID:    "user-1",
		},
	}
	want := []*internal_article_port.UnsummarizedArticle{
		{
			ID:        "art-1",
			Title:     "T",
			Content:   "C",
			URL:       "https://example.com/1",
			CreatedAt: now,
			UserID:    "user-1",
		},
	}

	got := toPortUnsummarizedArticles(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toPortUnsummarizedArticles() = %+v, want %+v", got, want)
	}
}
