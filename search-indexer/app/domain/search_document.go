package domain

type SearchDocument struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	UserID  string   `json:"user_id"`
}

func NewSearchDocument(article *Article) SearchDocument {
	return SearchDocument{
		ID:      article.ID(),
		Title:   article.Title(),
		Content: article.Content(),
		Tags:    article.Tags(),
		UserID:  article.UserID(),
	}
}
