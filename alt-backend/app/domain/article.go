package domain

type ArticleContent struct {
	ID      string
	Title   string
	Content string
	URL     string
}

// ArticleHead stores the <head> section and extracted OGP metadata for an article.
type ArticleHead struct {
	ID         string
	ArticleID  string
	HeadHTML   string
	OgImageURL string
}
