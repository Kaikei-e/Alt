package models

type SearchArticlesAPIResponse struct {
	Query string              `json:"query"`
	Hits  []SearchArticlesHit `json:"hits"`
}

type SearchArticlesHit struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}
