package domain

import (
	"errors"
	"time"
)

type RecapGenre struct {
	Genre         string         `json:"genre"`
	Summary       string         `json:"summary"`
	TopTerms      []string       `json:"top_terms"`
	ArticleCount  int            `json:"article_count"`
	ClusterCount  int            `json:"cluster_count"`
	EvidenceLinks []EvidenceLink `json:"evidence_links"`
}

type EvidenceLink struct {
	ArticleID   string `json:"article_id"`
	Title       string `json:"title"`
	SourceURL   string `json:"source_url"`
	PublishedAt string `json:"published_at"`
	Lang        string `json:"lang"`
}

type RecapSummary struct {
	JobID         string       `json:"job_id"`
	ExecutedAt    time.Time    `json:"executed_at"`
	WindowStart   time.Time    `json:"window_start"`
	WindowEnd     time.Time    `json:"window_end"`
	TotalArticles int          `json:"total_articles"`
	Genres        []RecapGenre `json:"genres"`
}

var ErrRecapNotFound = errors.New("recap not found")
