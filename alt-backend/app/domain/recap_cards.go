package domain

// RecapCardsResponse represents the JSON response returned by recap-worker
// for GET /v1/recaps/3days/cards.
type RecapCardsResponse struct {
	Job   *RecapCardsJob `json:"job"`
	Cards []*RecapCard   `json:"cards"`
}

// RecapCardsJob represents metadata of the 3-day cards generation job.
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
