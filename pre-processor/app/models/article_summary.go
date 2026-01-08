package models

import (
	"time"
)

type ArticleSummary struct {
	CreatedAt       time.Time `db:"created_at"`
	ID              string    `db:"id"`
	ArticleID       string    `db:"article_id"`
	UserID          string    `db:"user_id"`
	ArticleTitle    string    `db:"article_title"`
	SummaryJapanese string    `db:"summary_japanese"`
}
