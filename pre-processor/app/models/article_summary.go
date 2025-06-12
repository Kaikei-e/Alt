package models

import (
	"time"
)

type ArticleSummary struct {
	ID              string    `db:"id"`
	ArticleID       string    `db:"article_id"`
	ArticleTitle    string    `db:"article_title"`
	SummaryJapanese string    `db:"summary_japanese"`
	CreatedAt       time.Time `db:"created_at"`
}
