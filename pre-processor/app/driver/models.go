package driver

// ArticleWithSummary represents an article with its summary for quality checking.
type ArticleWithSummary struct {
	ArticleID       string `db:"article_id"`
	ArticleTitle    string `db:"title"`
	Content         string `db:"content"`
	SummaryJapanese string `db:"summary_japanese"`
	SummaryID       string `db:"summary_id"`
}
