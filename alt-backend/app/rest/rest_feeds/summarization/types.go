package summarization

// FeedSummarizePayload represents the request body for stream summarization
type FeedSummarizePayload struct {
	FeedURL   string `json:"feed_url"`
	ArticleID string `json:"article_id"`
	Content   string `json:"content"`
	Title     string `json:"title"`
}
