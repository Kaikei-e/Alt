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

type ArchiveArticleRequest struct {
	FeedURL string `json:"feed_url" validate:"required,url"`
	Title   string `json:"title,omitempty"`
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

// ArticlesWithCursorResponse represents the paginated response for articles with cursor
type ArticlesWithCursorResponse struct {
	Data       []ArticleResponse `json:"data"`
	NextCursor *string           `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
}

// ArticleResponse represents a single article in the response
type ArticleResponse struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Content     string   `json:"content"`
	PublishedAt string   `json:"published_at"`
	Tags        []string `json:"tags"`
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
	ArticleURL  string `json:"article_url"`
	Title       string `json:"title"`
	Author      string `json:"author,omitempty"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	PublishedAt string `json:"published_at"`
	FetchedAt   string `json:"fetched_at"`
	InoreaderID string `json:"inoreader_id"`
}

type FeedSummaryProvidedResponse struct {
	MatchedArticles []InoreaderSummaryResponse `json:"matched_articles"`
	TotalMatched    int                        `json:"total_matched"`
	RequestedCount  int                        `json:"requested_count"`
}

// ArticleInfo holds information about an article during batch processing
type ArticleInfo struct {
	URL    string
	ID     string
	Title  string
	Exists bool
	Error  error
}

// ImageFetchRequest represents the request payload for image fetching endpoint
type ImageFetchRequest struct {
	URL     string             `json:"url" validate:"required,url"`
	Options *ImageFetchOptions `json:"options,omitempty"`
}

// ImageFetchOptions represents optional parameters for image fetching
type ImageFetchOptions struct {
	MaxSize int `json:"max_size,omitempty" validate:"omitempty,min=1,max=10485760"` // 10MB max
	Timeout int `json:"timeout,omitempty" validate:"omitempty,min=1,max=60000"`     // 60 seconds max
}

// ImageFetchErrorResponse represents error response for image fetching
type ImageFetchErrorResponse struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type RecapRangeResponse struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type RecapArticleResponse struct {
	ArticleID   string  `json:"article_id"`
	Title       *string `json:"title,omitempty"`
	FullText    string  `json:"fulltext"`
	PublishedAt *string `json:"published_at,omitempty"`
	SourceURL   *string `json:"source_url,omitempty"`
	LangHint    *string `json:"lang_hint,omitempty"`
}

type RecapArticlesResponse struct {
	Range    RecapRangeResponse     `json:"range"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	HasMore  bool                   `json:"has_more"`
	Articles []RecapArticleResponse `json:"articles"`
}
