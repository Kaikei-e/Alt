package domain

type ArticleContent struct {
	ID      string
	Title   string
	Content string
	URL     string
	FeedID  string
}

// ArticleSource is what a caller holding only an article id needs in order to
// render or navigate to it: where the article lives and what it is called.
// A zero value is the miss — both fields come from one row, so a present URL
// with an empty title means the row itself has no title.
//
// Distinct from ArticleRef, which is the recall rail's projection fallback and
// carries a publication date it reads from a different query.
type ArticleSource struct {
	URL   string
	Title string
}

// ArticleHead stores the <head> section and extracted OGP metadata for an article.
type ArticleHead struct {
	ID         string
	ArticleID  string
	HeadHTML   string
	OgImageURL string
}
