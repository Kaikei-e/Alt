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

type FeedSearchPayload struct {
	Query string `json:"query" validate:"required,min=1,max=1000"`
}

type SearchArticlesResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}
