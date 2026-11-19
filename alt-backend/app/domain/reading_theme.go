package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidThemeInput = errors.New("invalid theme input")
	ErrThemeNotFound     = errors.New("theme not found")
)

type ReadingThemeSummary struct {
	ThemeID           uuid.UUID
	Title             string
	LatestVersion     int32
	ArticleCount      int32
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastOpenedAt      *time.Time
	LastOpenedVersion int32
}

type ThemeSentence struct {
	Text             string
	SourceArticleIDs []string
}

type ThemeTimelineEntry struct {
	Date             string
	Text             string
	SourceArticleIDs []string
}

type ThemeSource struct {
	ArticleID string
	Title     string
	URL       string
}

type ThemeVersionRef struct {
	Version      int32
	CreatedAt    time.Time
	ArticleCount int32
}

type ReadingThemePage struct {
	ThemeID       string
	Title         string
	Version       int32
	LatestVersion int32
	CreatedAt     time.Time
	KeyPoints     []ThemeSentence
	Timeline      []ThemeTimelineEntry
	WhatChanged   []ThemeSentence
	OpenQuestions []ThemeSentence
	Sources       []ThemeSource
	Versions      []ThemeVersionRef
}
