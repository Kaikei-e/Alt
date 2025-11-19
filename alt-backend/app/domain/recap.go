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
	JobID         string        `json:"job_id"`
	ExecutedAt    time.Time     `json:"executed_at"`
	WindowStart   time.Time     `json:"window_start"`
	WindowEnd     time.Time     `json:"window_end"`
	TotalArticles int           `json:"total_articles"`
	Genres        []RecapGenre  `json:"genres"`
	ClusterDraft  *ClusterDraft `json:"cluster_draft,omitempty"`
}

type ClusterDraft struct {
	ID           string         `json:"draft_id"`
	Description  string         `json:"description"`
	Source       string         `json:"source,omitempty"`
	GeneratedAt  time.Time      `json:"generated_at"`
	TotalEntries int            `json:"total_entries"`
	Genres       []ClusterGenre `json:"genres"`
}

type ClusterGenre struct {
	Genre        string           `json:"genre"`
	SampleSize   int              `json:"sample_size"`
	ClusterCount int              `json:"cluster_count"`
	Clusters     []ClusterSegment `json:"clusters"`
}

type ClusterSegment struct {
	ClusterID                string           `json:"cluster_id"`
	Label                    string           `json:"label"`
	Count                    int              `json:"count"`
	MarginMean               float64          `json:"margin_mean"`
	MarginStd                float64          `json:"margin_std"`
	TopBoostMean             float64          `json:"top_boost_mean"`
	GraphBoostAvailableRatio float64          `json:"graph_boost_available_ratio"`
	TagCountMean             float64          `json:"tag_count_mean"`
	TagEntropyMean           float64          `json:"tag_entropy_mean"`
	TopTags                  []string         `json:"top_tags"`
	RepresentativeArticles   []ClusterArticle `json:"representative_articles"`
}

type ClusterArticle struct {
	ArticleID      string   `json:"article_id"`
	Margin         float64  `json:"margin"`
	TopBoost       float64  `json:"top_boost"`
	Strategy       string   `json:"strategy"`
	TagCount       int      `json:"tag_count"`
	CandidateCount int      `json:"candidate_count"`
	TopTags        []string `json:"top_tags"`
}

var ErrRecapNotFound = errors.New("recap not found")
