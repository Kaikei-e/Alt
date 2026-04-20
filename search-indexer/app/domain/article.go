package domain

import (
	"errors"
	"time"
)

type Article struct {
	id          string
	title       string
	content     string
	tags        []string
	createdAt   time.Time
	userID      string
	language    string
	publishedAt time.Time
}

func NewArticle(id, title, content string, tags []string, createdAt time.Time, userID string) (*Article, error) {
	return NewArticleWithPublishedAt(id, title, content, tags, createdAt, userID, time.Time{})
}

// NewArticleWithPublishedAt mirrors NewArticle and also carries the
// publication timestamp through the pipeline so Meilisearch documents can
// be filtered by date window. A zero publishedAt means "unknown" and
// receives no filter participation downstream.
func NewArticleWithPublishedAt(
	id, title, content string,
	tags []string,
	createdAt time.Time,
	userID string,
	publishedAt time.Time,
) (*Article, error) {
	if id == "" {
		return nil, errors.New("article ID cannot be empty")
	}
	if title == "" {
		return nil, errors.New("article title cannot be empty")
	}

	return &Article{
		id:          id,
		title:       title,
		content:     content,
		tags:        tags,
		createdAt:   createdAt,
		userID:      userID,
		publishedAt: publishedAt,
	}, nil
}

func (a *Article) ID() string {
	return a.id
}

func (a *Article) Title() string {
	return a.title
}

func (a *Article) Content() string {
	return a.content
}

func (a *Article) Tags() []string {
	return a.tags
}

func (a *Article) CreatedAt() time.Time {
	return a.createdAt
}

func (a *Article) UserID() string {
	return a.userID
}

func (a *Article) Language() string {
	return a.language
}

func (a *Article) SetLanguage(lang string) {
	a.language = lang
}

// PublishedAt returns the source publication timestamp. Zero value means
// the upstream feed did not supply one.
func (a *Article) PublishedAt() time.Time {
	return a.publishedAt
}

// SetPublishedAt lets the backfill / event handler populate the timestamp
// when the ingest path did not provide one at construction time.
func (a *Article) SetPublishedAt(t time.Time) {
	a.publishedAt = t
}

func (a *Article) HasTag(tag string) bool {
	if tag == "" {
		return false
	}

	for _, t := range a.tags {
		if t == tag {
			return true
		}
	}
	return false
}
