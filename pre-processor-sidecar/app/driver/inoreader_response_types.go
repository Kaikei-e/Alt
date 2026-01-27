// ABOUTME: Inoreader API公式レスポンス構造体定義 - Driver Layer
// ABOUTME: 型安全なJSONバインディングのための正確なresponse model定義

package driver

import "time"

// StreamContentsResponse represents the complete response structure from Inoreader stream contents API
// Based on official documentation: https://www.inoreader.com/developers/stream-contents
type StreamContentsResponse struct {
	Direction   string `json:"direction"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Self        struct {
		Href string `json:"href"`
	} `json:"self"`
	Updated      int64                  `json:"updated"`
	UpdatedUsec  string                 `json:"updatedUsec"`
	Items        []InoreaderArticleItem `json:"items"`
	Continuation string                 `json:"continuation"`
}

// InoreaderArticleItem represents individual article item from Inoreader API response
type InoreaderArticleItem struct {
	CrawlTimeMsec string                `json:"crawlTimeMsec"`
	TimestampUsec string                `json:"timestampUsec"`
	ID            string                `json:"id"`
	Categories    []string              `json:"categories"`
	Title         string                `json:"title"`
	Published     int64                 `json:"published"`
	Updated       int64                 `json:"updated"`
	Canonical     []InoreaderLink       `json:"canonical"`
	Alternate     []InoreaderLink       `json:"alternate"`
	Summary       InoreaderSummary      `json:"summary"`
	Author        string                `json:"author"`
	Origin        InoreaderOrigin       `json:"origin"`
	Annotations   []InoreaderAnnotation `json:"annotations,omitempty"`
	// Additional fields that might be present
	LikingUsers []interface{} `json:"likingUsers,omitempty"`
	Comments    []interface{} `json:"comments,omitempty"`
	CommentsNum int           `json:"commentsNum,omitempty"`
}

// InoreaderLink represents canonical/alternate links in article
type InoreaderLink struct {
	Href string `json:"href"`
	Type string `json:"type,omitempty"`
}

// InoreaderSummary represents the summary field containing article content
// This is the key structure for content extraction
type InoreaderSummary struct {
	Direction string `json:"direction"` // "ltr" or "rtl"
	Content   string `json:"content"`   // HTML content of the article
}

// InoreaderOrigin represents the origin feed information
type InoreaderOrigin struct {
	StreamID string `json:"streamId"` // e.g., "feed/https://example.com/rss"
	Title    string `json:"title"`    // Feed title
	HtmlUrl  string `json:"htmlUrl"`  // Website URL
}

// InoreaderAnnotation represents user annotations if annotations=1 parameter is used
type InoreaderAnnotation struct {
	ID      int64  `json:"id"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	AddedOn int64  `json:"added_on"`
	Text    string `json:"text"`
	Note    string `json:"note"`
}

// SubscriptionListResponse represents the response structure from subscription list API
type SubscriptionListResponse struct {
	Subscriptions []InoreaderSubscriptionItem `json:"subscriptions"`
}

// InoreaderSubscriptionItem represents individual subscription from subscription list API
type InoreaderSubscriptionItem struct {
	ID         string              `json:"id"`         // e.g., "feed/http://example.com/rss"
	Title      string              `json:"title"`      // Feed title
	Categories []InoreaderCategory `json:"categories"` // Folder/label information
	URL        string              `json:"url"`        // XML feed URL
	HtmlUrl    string              `json:"htmlUrl"`    // Website URL
	IconUrl    string              `json:"iconUrl"`    // Favicon URL
}

// InoreaderCategory represents a category/folder in Inoreader
type InoreaderCategory struct {
	ID    string `json:"id"`    // e.g., "user/1234/label/News"
	Label string `json:"label"` // Display name
}

// Helper methods for time conversion
func (item *InoreaderArticleItem) GetPublishedTime() time.Time {
	return time.Unix(item.Published, 0)
}

func (item *InoreaderArticleItem) GetUpdatedTime() time.Time {
	return time.Unix(item.Updated, 0)
}

func (response *StreamContentsResponse) GetUpdatedTime() time.Time {
	return time.Unix(response.Updated, 0)
}

// Helper methods for content validation
func (summary *InoreaderSummary) HasContent() bool {
	return len(summary.Content) > 0
}

func (summary *InoreaderSummary) IsRTL() bool {
	return summary.Direction == "rtl"
}

func (item *InoreaderArticleItem) GetCanonicalURL() string {
	if len(item.Canonical) > 0 {
		return item.Canonical[0].Href
	}
	return ""
}

func (item *InoreaderArticleItem) GetOriginStreamID() string {
	return item.Origin.StreamID
}
