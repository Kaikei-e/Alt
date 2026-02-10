package domain

import "time"

type RSSFeed struct {
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Link          string            `json:"link"`
	FeedLink      string            `json:"feedLink"`
	Links         []string          `json:"links"`
	Updated       string            `json:"updated"`
	UpdatedParsed time.Time         `json:"updatedParsed"`
	Language      string            `json:"language"`
	Image         RSSFeedImage      `json:"image"`
	Generator     string            `json:"generator"`
	Extensions    RSSFeedExtensions `json:"extensions"`
	Items         []FeedItem        `json:"items"`
	FeedType      string            `json:"feedType"`
	FeedVersion   string            `json:"feedVersion"`
}

type RSSFeedExtensions struct {
	Atom Atom `json:"atom"`
}

type Atom struct {
	Link []Link `json:"link"`
}

type Link struct {
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	Attrs    Attrs    `json:"attrs"`
	Children Children `json:"children"`
}

type Attrs struct {
	Href string `json:"href"`
	Rel  string `json:"rel"`
	Type string `json:"type"`
}

type Children struct {
}

type RSSFeedImage struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type FeedItem struct {
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Link            string    `json:"link"`
	Links           []string  `json:"links"`
	Published       string    `json:"published"`
	PublishedParsed time.Time `json:"publishedParsed"`
	Author          Author    `json:"author"`
	Authors         []Author  `json:"authors"`
	// ArticleID is the ID of the corresponding article in the articles table.
	// Empty string if no article exists for this feed item.
	ArticleID string `json:"articleId,omitempty"`
	// IsRead indicates whether this feed has been read by the current user.
	IsRead bool `json:"isRead,omitempty"`
	// FeedLinkID is the feed_links.id for the RSS source this item belongs to.
	FeedLinkID *string `json:"feedLinkId,omitempty"`
}

type Author struct {
	Name string `json:"name"`
}

type DcEXT struct {
	Creator []string `json:"creator"`
}

type Enclosure struct {
	URL    string `json:"url"`
	Length string `json:"length"`
	Type   Type   `json:"type"`
}

type ItemExtensions struct {
	Dc Dc `json:"dc"`
}

type Dc struct {
	Creator []CreatorElement `json:"creator"`
}

type CreatorElement struct {
	Name     Name     `json:"name"`
	Value    string   `json:"value"`
	Attrs    Children `json:"attrs"`
	Children Children `json:"children"`
}

type ItemImage struct {
	URL string `json:"url"`
}

type Type string

const (
	ImagePNG Type = "image/png"
)

type Name string

const (
	Creator Name = "creator"
)

type SearchArticleHit struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

type SearchIndexerArticleHit struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}
