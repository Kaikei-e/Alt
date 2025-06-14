package rest

type RssFeedLink struct {
	URL string `json:"url"`
}

type ReadStatus struct {
	FeedURL string `json:"feed_url"`
}

type FeedUrlPayload struct {
	FeedURL string `json:"feed_url"`
}

type FeedStatsSummary struct {
	FeedAmount           feedAmount           `json:"feed_amount"`
	SummarizedFeedAmount summarizedFeedAmount `json:"summarized_feed"`
}

type feedAmount struct {
	Amount int `json:"amount"`
}

type summarizedFeedAmount struct {
	Amount int `json:"amount"`
}
