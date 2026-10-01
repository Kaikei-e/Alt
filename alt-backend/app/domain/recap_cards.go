package domain

// RecapCardsResponse represents the JSON response returned by recap-worker
// for GET /v1/topic-cards.
type RecapCardsResponse struct {
	Job       *RecapCardsJob `json:"job"`
	Cards     []*RecapCard   `json:"cards"`
	LatestRun *RecapCardsRun `json:"latest_run"`
}

// RecapCardsRun represents metadata of a topic cards generation run.
type RecapCardsRun struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"` // pending | running | completed | failed
	KickedAt  string `json:"kicked_at"`
	UpdatedAt string `json:"updated_at"`
}

// RecapCardsJob represents metadata of the topic cards generation job.
type RecapCardsJob struct {
	JobID         string `json:"job_id"`
	KickedAt      string `json:"kicked_at"`
	From          string `json:"from"`
	To            string `json:"to"`
	ParamsVersion string `json:"params_version"`
	CardsSelected int    `json:"cards_selected"`
	Degraded      bool   `json:"degraded"`
}

// RecapCard represents an individual topic recap card.
type RecapCard struct {
	ID              string             `json:"id"`
	Rank            int                `json:"rank"`
	StoryID         string             `json:"story_id"`
	ContinuesCardID *string            `json:"continues_card_id"`
	HeadlineJa      string             `json:"headline_ja"`
	WhatJa          string             `json:"what_ja"`
	WhyJa           *string            `json:"why_ja"`
	Genre           *string            `json:"genre"`
	Sources         []*RecapCardSource `json:"sources"`
	CreatedAt       string             `json:"created_at"`
}

// RecapCardSource represents a cited source feed item within a recap card.
type RecapCardSource struct {
	N       int     `json:"n"`
	FeedID  string  `json:"feed_id"`
	URL     string  `json:"url"`
	Host    string  `json:"host"`
	Title   string  `json:"title"`
	PubDate *string `json:"pub_date"`
}
