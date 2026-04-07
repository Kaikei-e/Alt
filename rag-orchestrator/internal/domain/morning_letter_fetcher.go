package domain

import "context"

// MorningLetterDoc represents a morning letter document for chat context grounding.
type MorningLetterDoc struct {
	Lead     string
	Sections []MorningLetterDocSection
}

// MorningLetterDocSection represents a section in a morning letter.
type MorningLetterDocSection struct {
	Key     string
	Title   string
	Bullets []string
}

// MorningLetterFetcher fetches morning letter documents from recap-worker.
type MorningLetterFetcher interface {
	// FetchLatest returns the most recent morning letter, or nil if none exists.
	FetchLatest(ctx context.Context) (*MorningLetterDoc, error)
	// FetchByDate returns the letter for a specific civil date, or nil if not found.
	FetchByDate(ctx context.Context, targetDate string) (*MorningLetterDoc, error)
}
