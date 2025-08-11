package rest

type RssFeedLink struct {
	URL string `json:"url" validate:"required,url"`
}

type ReadStatus struct {
	FeedURL string `json:"feed_url" validate:"required,url"`
}

type FeedUrlPayload struct {
	FeedURL string `json:"feed_url" validate:"required,url"`
}

type FeedStatsSummary struct {
	FeedAmount           feedAmount           `json:"feed_amount"`
	SummarizedFeedAmount summarizedFeedAmount `json:"summarized_feed"`
}

type UnsummarizedFeedStatsSummary struct {
	FeedAmount             feedAmount             `json:"feed_amount"`
	UnsummarizedFeedAmount unsummarizedFeedAmount `json:"unsummarized_feed"`
	ArticleAmount          articleAmount          `json:"total_articles,omitempty"`
}

type feedAmount struct {
	Amount int `json:"amount"`
}

type summarizedFeedAmount struct {
	Amount int `json:"amount"`
}

type unsummarizedFeedAmount struct {
	Amount int `json:"amount"`
}

type articleAmount struct {
	Amount int `json:"amount"`
}

type FeedSearchPayload struct {
	Query string `json:"query" validate:"required,min=1,max=1000"`
}

type FeedTagsPayload struct {
	FeedURL string `json:"feed_url" validate:"required,uri"`
	Limit   int    `json:"limit,omitempty" validate:"omitempty,min=1,max=100"`
	Cursor  string `json:"cursor,omitempty"` // RFC3339 format timestamp
}

type SearchArticlesResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type FeedTagsResponse struct {
	FeedID string            `json:"feed_id"`
	Tags   []FeedTagResponse `json:"tags"`
}

type FeedTagResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type FeedSummaryRequest struct {
	FeedURLs []string `json:"feed_urls" validate:"required,min=1,max=50,dive,required,url"`
}

type InoreaderSummaryResponse struct {
	ArticleURL     string `json:"article_url"`
	Title          string `json:"title"`
	Author         string `json:"author,omitempty"`
	Content        string `json:"content"`
	ContentType    string `json:"content_type"`
	PublishedAt    string `json:"published_at"`
	FetchedAt      string `json:"fetched_at"`
	InoreaderID    string `json:"inoreader_id"`
}

type FeedSummaryProvidedResponse struct {
	MatchedArticles []InoreaderSummaryResponse `json:"matched_articles"`
	TotalMatched    int                        `json:"total_matched"`
	RequestedCount  int                        `json:"requested_count"`
}
