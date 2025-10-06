package domain

import (
	"errors"
	"time"
)

type Article struct {
	id        string
	title     string
	content   string
	tags      []string
	createdAt time.Time
	userID    string
}

func NewArticle(id, title, content string, tags []string, createdAt time.Time, userID string) (*Article, error) {
	if id == "" {
		return nil, errors.New("article ID cannot be empty")
	}
	if title == "" {
		return nil, errors.New("article title cannot be empty")
	}

	return &Article{
		id:        id,
		title:     title,
		content:   content,
		tags:      tags,
		createdAt: createdAt,
		userID:    userID,
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
