package domain

import (
	"testing"
	"time"
)

func TestArticle_NewArticle(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		title     string
		content   string
		tags      []string
		createdAt time.Time
		userID    string
		wantErr   bool
	}{
		{
			name:      "valid article",
			id:        "article-1",
			title:     "Test Article",
			content:   "This is test content",
			tags:      []string{"tag1", "tag2"},
			createdAt: time.Now(),
			userID:    "user-123",
			wantErr:   false,
		},
		{
			name:      "valid article with empty userID",
			id:        "article-2",
			title:     "Test Article",
			content:   "This is test content",
			tags:      []string{"tag1"},
			createdAt: time.Now(),
			userID:    "",
			wantErr:   false,
		},
		{
			name:      "empty id should fail",
			id:        "",
			title:     "Test Article",
			content:   "This is test content",
			tags:      []string{"tag1"},
			createdAt: time.Now(),
			userID:    "user-123",
			wantErr:   true,
		},
		{
			name:      "empty title should fail",
			id:        "article-1",
			title:     "",
			content:   "This is test content",
			tags:      []string{"tag1"},
			createdAt: time.Now(),
			userID:    "user-123",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			article, err := NewArticle(tt.id, tt.title, tt.content, tt.tags, tt.createdAt, tt.userID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewArticle() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("NewArticle() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if article.ID() != tt.id {
				t.Errorf("Article.ID() = %v, want %v", article.ID(), tt.id)
			}
			if article.Title() != tt.title {
				t.Errorf("Article.Title() = %v, want %v", article.Title(), tt.title)
			}
			if article.Content() != tt.content {
				t.Errorf("Article.Content() = %v, want %v", article.Content(), tt.content)
			}
			if article.UserID() != tt.userID {
				t.Errorf("Article.UserID() = %v, want %v", article.UserID(), tt.userID)
			}
		})
	}
}

func TestArticle_HasTag(t *testing.T) {
	article, err := NewArticle("1", "Test", "Content", []string{"tag1", "tag2"}, time.Now(), "user-123")
	if err != nil {
		t.Fatalf("NewArticle() error = %v", err)
	}

	tests := []struct {
		name string
		tag  string
		want bool
	}{
		{"existing tag", "tag1", true},
		{"non-existing tag", "tag3", false},
		{"empty tag", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := article.HasTag(tt.tag); got != tt.want {
				t.Errorf("Article.HasTag() = %v, want %v", got, tt.want)
			}
		})
	}
}
