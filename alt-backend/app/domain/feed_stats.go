package domain

type FeedStats struct {
	TotalFeeds                int64 `json:"total_feeds"`
	TotalArticles             int64 `json:"total_articles"`
	TotalUnsummarizedArticles int64 `json:"total_unsummarized_articles"`
	TotalSummarizedArticles   int64 `json:"total_summarized_articles"`
}
