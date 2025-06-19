package domain

type SearchDocument struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

func NewSearchDocument(article *Article) SearchDocument {
	return SearchDocument{
		ID:      article.ID(),
		Title:   article.Title(),
		Content: article.Content(),
		Tags:    article.Tags(),
	}
}